package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// 分享服务返回的错误。
var (
	// ErrShareNotFound 表示分享不存在。
	ErrShareNotFound = errors.New("service: share not found")
	// ErrShareUnavailable 表示分享已禁用、过期或达到访问上限。
	ErrShareUnavailable = errors.New("service: share is no longer available")
	// ErrSharePasswordRequired 表示该分享需要密码。
	ErrSharePasswordRequired = errors.New("service: share password required")
	// ErrShareInvalidPassword 表示分享密码错误。
	ErrShareInvalidPassword = errors.New("service: invalid share password")
)

// maxShareImages 限制相册分享一次返回的图片数量。
const maxShareImages = 500

// ShareDTO 是分享链接在 API 中的表示形式。
type ShareDTO struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	URL         string     `json:"url"`
	TargetType  string     `json:"target_type"`
	TargetID    string     `json:"target_id"`
	HasPassword bool       `json:"has_password"`
	Disabled    bool       `json:"disabled"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxViews    int64      `json:"max_views"`
	ViewCount   int64      `json:"view_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ShareInput 是创建分享的输入。
type ShareInput struct {
	TargetType     string
	TargetID       string
	Password       string
	ExpiresInHours int
	MaxViews       int64
}

// SharePayloadDTO 是通过密码校验后返回的分享内容。
type SharePayloadDTO struct {
	Share  ShareDTO   `json:"share"`
	Image  *ImageDTO  `json:"image,omitempty"`
	Album  *AlbumDTO  `json:"album,omitempty"`
	Images []ImageDTO `json:"images,omitempty"`
}

// ShareService 管理分享链接的创建、访问与撤销。
type ShareService struct {
	repo    *store.Repository
	baseURL string
}

// NewShareService 构造一个 ShareService。baseURL 用于拼接完整分享链接。
func NewShareService(repo *store.Repository, baseURL string) *ShareService {
	return &ShareService{repo: repo, baseURL: strings.TrimRight(baseURL, "/")}
}

// Create 为一张图片或一个相册创建分享链接，并强制校验所有权。
func (s *ShareService) Create(ctx context.Context, principal *auth.Principal, in ShareInput) (*ShareDTO, error) {
	targetType := strings.TrimSpace(in.TargetType)
	targetID := strings.TrimSpace(in.TargetID)
	if targetID == "" {
		return nil, fmt.Errorf("%w: target id is required", ErrInvalidInput)
	}
	switch targetType {
	case store.ShareTargetImage:
		image, err := s.repo.GetByID(ctx, targetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("create share: %w", err)
		}
		if !canAccess(principal, image.UserID) {
			return nil, ErrForbidden
		}
	case store.ShareTargetAlbum:
		album, err := s.repo.GetAlbumByID(ctx, targetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrAlbumNotFound
			}
			return nil, fmt.Errorf("create share: %w", err)
		}
		if !canAccess(principal, album.UserID) {
			return nil, ErrForbidden
		}
	default:
		return nil, fmt.Errorf("%w: target type must be image or album", ErrInvalidInput)
	}
	if in.MaxViews < 0 {
		return nil, fmt.Errorf("%w: max views must not be negative", ErrInvalidInput)
	}
	if in.ExpiresInHours < 0 {
		return nil, fmt.Errorf("%w: expiry must not be negative", ErrInvalidInput)
	}

	token, err := newShareToken()
	if err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}
	passwordHash := ""
	if password := strings.TrimSpace(in.Password); password != "" {
		passwordHash, err = auth.HashPassword(password)
		if err != nil {
			return nil, fmt.Errorf("create share: %w", err)
		}
	}
	share := &store.Share{
		ID:           uuid.NewString(),
		Token:        token,
		UserID:       principal.UserID,
		TargetType:   targetType,
		TargetID:     targetID,
		PasswordHash: passwordHash,
		MaxViews:     in.MaxViews,
	}
	if in.ExpiresInHours > 0 {
		expires := time.Now().Add(time.Duration(in.ExpiresInHours) * time.Hour)
		share.ExpiresAt = &expires
	}
	if err := s.repo.CreateShare(ctx, share); err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}
	return s.toDTO(share), nil
}

// List 返回某个主体创建的分享。
func (s *ShareService) List(ctx context.Context, principal *auth.Principal) ([]ShareDTO, error) {
	shares, err := s.repo.ListSharesByUser(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	dtos := make([]ShareDTO, 0, len(shares))
	for i := range shares {
		dtos = append(dtos, *s.toDTO(&shares[i]))
	}
	return dtos, nil
}

// Revoke 删除某个主体拥有的分享。
func (s *ShareService) Revoke(ctx context.Context, principal *auth.Principal, id string) error {
	if err := s.repo.DeleteShare(ctx, principal.UserID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrShareNotFound
		}
		return fmt.Errorf("revoke share: %w", err)
	}
	return nil
}

// Info 返回分享的公开元信息（不含内容），用于判断是否需要密码。
func (s *ShareService) Info(ctx context.Context, token string) (*ShareDTO, error) {
	share, err := s.load(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := shareAvailable(share); err != nil {
		return nil, err
	}
	return s.toDTO(share), nil
}

// Access 校验密码与可用性后返回分享内容，并计入一次访问。
func (s *ShareService) Access(ctx context.Context, token, password string) (*SharePayloadDTO, error) {
	share, err := s.load(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := shareAvailable(share); err != nil {
		return nil, err
	}
	if share.PasswordHash != "" {
		if strings.TrimSpace(password) == "" {
			return nil, ErrSharePasswordRequired
		}
		if !auth.VerifyPassword(share.PasswordHash, password) {
			return nil, ErrShareInvalidPassword
		}
	}
	allowed, err := s.repo.ConsumeShareView(ctx, share.ID)
	if err != nil {
		return nil, fmt.Errorf("access share: %w", err)
	}
	if !allowed {
		return nil, ErrShareUnavailable
	}
	share.ViewCount++
	payload, err := s.buildPayload(ctx, share)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// load 按 token 加载分享，并把未找到映射为 ErrShareNotFound。
func (s *ShareService) load(ctx context.Context, token string) (*store.Share, error) {
	share, err := s.repo.GetShareByToken(ctx, strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("load share: %w", err)
	}
	return share, nil
}

// buildPayload 组装分享指向的内容。
func (s *ShareService) buildPayload(ctx context.Context, share *store.Share) (*SharePayloadDTO, error) {
	payload := &SharePayloadDTO{Share: *s.toDTO(share)}
	switch share.TargetType {
	case store.ShareTargetImage:
		image, err := s.repo.GetByID(ctx, share.TargetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrShareNotFound
			}
			return nil, fmt.Errorf("share payload: %w", err)
		}
		payload.Image = toDTO(image)
	case store.ShareTargetAlbum:
		album, err := s.repo.GetAlbumByID(ctx, share.TargetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrShareNotFound
			}
			return nil, fmt.Errorf("share payload: %w", err)
		}
		ownerName := ""
		if album.UserID != nil {
			if account, accErr := s.repo.GetAccountByID(ctx, store.RoleCustomer, *album.UserID); accErr == nil {
				ownerName = account.Username
			}
		}
		count, err := s.repo.CountImagesInAlbum(ctx, album.ID)
		if err != nil {
			return nil, fmt.Errorf("share payload: %w", err)
		}
		payload.Album = albumToDTO(album, count, ownerName)

		images, _, err := s.repo.ListImages(ctx, store.ImageListOptions{
			AlbumID: &album.ID,
			Limit:   maxShareImages,
			Order:   "newest",
		})
		if err != nil {
			return nil, fmt.Errorf("share payload: %w", err)
		}
		dtos := make([]ImageDTO, 0, len(images))
		for i := range images {
			dto := toDTO(&images[i])
			dto.OwnerUsername = ownerName
			dtos = append(dtos, *dto)
		}
		payload.Images = dtos
	default:
		return nil, ErrShareNotFound
	}
	return payload, nil
}

// toDTO 组装分享的对外表示。
func (s *ShareService) toDTO(share *store.Share) *ShareDTO {
	return &ShareDTO{
		ID:          share.ID,
		Token:       share.Token,
		URL:         s.baseURL + "/s/" + share.Token,
		TargetType:  share.TargetType,
		TargetID:    share.TargetID,
		HasPassword: share.PasswordHash != "",
		Disabled:    share.Disabled,
		ExpiresAt:   share.ExpiresAt,
		MaxViews:    share.MaxViews,
		ViewCount:   share.ViewCount,
		CreatedAt:   share.CreatedAt,
	}
}

// shareAvailable 报告分享当前是否可用。
func shareAvailable(share *store.Share) error {
	if share.Disabled {
		return ErrShareUnavailable
	}
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return ErrShareUnavailable
	}
	if share.MaxViews > 0 && share.ViewCount >= share.MaxViews {
		return ErrShareUnavailable
	}
	return nil
}

// newShareToken 生成一个不可猜测的 URL 安全令牌。
func newShareToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
