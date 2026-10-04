package service

import (
	"context"
	"fmt"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/store"
)

// StatsDTO summarizes instance-wide metrics.
type StatsDTO struct {
	Admins        int64    `json:"admins"`
	Customers     int64    `json:"customers"`
	Users         int64    `json:"users"`
	Images        int64    `json:"images"`
	TotalBytes    int64    `json:"total_bytes"`
	StorageDriver string   `json:"storage_driver"`
	Processor     string   `json:"processor"`
	Formats       []string `json:"formats"`
}

// UpdateUserInput carries optional account changes.
type UpdateUserInput struct {
	Disabled *bool
}

// AdminService provides administrative operations over both account tables.
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
	admins, err := s.repo.CountAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: count admins: %w", err)
	}
	customers, err := s.repo.CountCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: count customers: %w", err)
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
		Admins:        admins,
		Customers:     customers,
		Users:         admins + customers,
		Images:        imageStats.Count,
		TotalBytes:    imageStats.TotalBytes,
		StorageDriver: s.storageDriver,
		Processor:     processorName,
		Formats:       formats,
	}, nil
}

// ListCustomers returns every customer account.
func (s *AdminService) ListCustomers(ctx context.Context) ([]UserDTO, error) {
	accounts, err := s.repo.ListCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return toUserDTOs(accounts), nil
}

// ListAdmins returns every admin account.
func (s *AdminService) ListAdmins(ctx context.Context) ([]UserDTO, error) {
	accounts, err := s.repo.ListAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	return toUserDTOs(accounts), nil
}

// RegisterAdmin creates an admin in the admins table.
func (s *AdminService) RegisterAdmin(ctx context.Context, accounts *AccountService, username, password string) (*UserDTO, error) {
	return accounts.RegisterAdmin(ctx, username, password)
}

// UpdateCustomer applies optional changes to a customer account.
func (s *AdminService) UpdateCustomer(ctx context.Context, id string, in UpdateUserInput) (*UserDTO, error) {
	account, err := s.repo.UpdateCustomer(ctx, id, store.UserUpdate{Disabled: in.Disabled})
	if err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}
	return toUserDTO(account), nil
}

// UpdateAdmin applies optional changes to an admin account. It refuses to
// disable the acting admin or the last enabled admin so the instance cannot be
// locked out.
func (s *AdminService) UpdateAdmin(ctx context.Context, actorID, id string, in UpdateUserInput) (*UserDTO, error) {
	target, err := s.repo.GetAccountByID(ctx, store.RoleAdmin, id)
	if err != nil {
		return nil, fmt.Errorf("update admin: %w", err)
	}
	disabling := in.Disabled != nil && *in.Disabled && !target.Disabled
	if disabling {
		if id == actorID {
			return nil, fmt.Errorf("%w: cannot disable your own account", ErrInvalidInput)
		}
		count, err := s.repo.CountEnabledAdmins(ctx)
		if err != nil {
			return nil, fmt.Errorf("update admin: count admins: %w", err)
		}
		if count <= 1 {
			return nil, fmt.Errorf("%w: cannot disable the last enabled admin", ErrInvalidInput)
		}
	}
	account, err := s.repo.UpdateAdmin(ctx, id, store.UserUpdate{Disabled: in.Disabled})
	if err != nil {
		return nil, fmt.Errorf("update admin: %w", err)
	}
	return toUserDTO(account), nil
}

// DeleteCustomer removes a customer account.
func (s *AdminService) DeleteCustomer(ctx context.Context, id string) error {
	if err := s.repo.DeleteAccount(ctx, store.RoleCustomer, id); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	return nil
}

// DeleteAdmin removes an admin account. It refuses to delete the acting admin or
// the last enabled admin.
func (s *AdminService) DeleteAdmin(ctx context.Context, actorID, id string) error {
	target, err := s.repo.GetAccountByID(ctx, store.RoleAdmin, id)
	if err != nil {
		return fmt.Errorf("delete admin: %w", err)
	}
	if id == actorID {
		return fmt.Errorf("%w: cannot delete your own account", ErrInvalidInput)
	}
	if !target.Disabled {
		count, err := s.repo.CountEnabledAdmins(ctx)
		if err != nil {
			return fmt.Errorf("delete admin: count admins: %w", err)
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot delete the last enabled admin", ErrInvalidInput)
		}
	}
	if err := s.repo.DeleteAccount(ctx, store.RoleAdmin, id); err != nil {
		return fmt.Errorf("delete admin: %w", err)
	}
	return nil
}

func toUserDTOs(accounts []store.Account) []UserDTO {
	dtos := make([]UserDTO, 0, len(accounts))
	for i := range accounts {
		dtos = append(dtos, *toUserDTO(&accounts[i]))
	}
	return dtos
}
