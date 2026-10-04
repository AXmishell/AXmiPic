package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrInvalidRole is returned when an unknown role is requested.
var ErrInvalidRole = errors.New("service: invalid role")

// StatsDTO summarizes instance-wide metrics.
type StatsDTO struct {
	Users         int64    `json:"users"`
	Images        int64    `json:"images"`
	TotalBytes    int64    `json:"total_bytes"`
	StorageDriver string   `json:"storage_driver"`
	Processor     string   `json:"processor"`
	Formats       []string `json:"formats"`
}

// UpdateUserInput carries optional account changes.
type UpdateUserInput struct {
	Role     *string
	Disabled *bool
}

// AdminService provides administrative operations.
type AdminService struct {
	repo          *store.Repository
	storageDriver string
	processor     imaging.Processor
}

// NewAdminService constructs an AdminService.
func NewAdminService(repo *store.Repository, storageDriver string, processor imaging.Processor) *AdminService {
	return &AdminService{repo: repo, storageDriver: storageDriver, processor: processor}
}

// Stats returns aggregate instance metrics.
func (s *AdminService) Stats(ctx context.Context) (*StatsDTO, error) {
	users, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: count users: %w", err)
	}
	imageStats, err := s.repo.ImageStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: image stats: %w", err)
	}

	formats := []string{}
	processorName := ""
	if s.processor != nil {
		capabilities := s.processor.Capabilities()
		processorName = capabilities.Name
		for _, f := range capabilities.OutputFormats {
			formats = append(formats, string(f))
		}
	}
	return &StatsDTO{
		Users:         users,
		Images:        imageStats.Count,
		TotalBytes:    imageStats.TotalBytes,
		StorageDriver: s.storageDriver,
		Processor:     processorName,
		Formats:       formats,
	}, nil
}

// ListUsers returns every account.
func (s *AdminService) ListUsers(ctx context.Context) ([]UserDTO, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	dtos := make([]UserDTO, 0, len(users))
	for i := range users {
		dtos = append(dtos, *toUserDTO(&users[i]))
	}
	return dtos, nil
}

// UpdateUser applies optional role and disabled changes. actorID is the admin
// performing the change and is used to prevent self-lockout.
func (s *AdminService) UpdateUser(ctx context.Context, actorID, id string, in UpdateUserInput) (*UserDTO, error) {
	if in.Role != nil {
		switch auth.Role(*in.Role) {
		case auth.RoleUser, auth.RoleAdmin:
		default:
			return nil, fmt.Errorf("%w: %q", ErrInvalidRole, *in.Role)
		}
	}

	target, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	demoting := in.Role != nil && auth.Role(*in.Role) != auth.RoleAdmin && target.Role == string(auth.RoleAdmin)
	disabling := in.Disabled != nil && *in.Disabled && !target.Disabled

	if id == actorID && (demoting || disabling) {
		return nil, fmt.Errorf("%w: cannot disable or demote your own account", ErrInvalidInput)
	}
	if target.Role == string(auth.RoleAdmin) && !target.Disabled && (demoting || disabling) {
		count, err := s.repo.CountEnabledAdmins(ctx)
		if err != nil {
			return nil, fmt.Errorf("update user: count admins: %w", err)
		}
		if count <= 1 {
			return nil, fmt.Errorf("%w: cannot remove the last enabled admin", ErrInvalidInput)
		}
	}

	user, err := s.repo.UpdateUser(ctx, id, store.UserUpdate{Role: in.Role, Disabled: in.Disabled})
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return toUserDTO(user), nil
}
