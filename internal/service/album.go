package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrAlbumNotFound 在请求的相册不存在时返回。
var ErrAlbumNotFound = errors.New("service: album not found")

// consts 约束相册字段长度。
const (
	maxAlbumNameRunes  = 100
	maxAlbumIntroRunes = 255
)

// AlbumDTO 是相册在 API 中的表示形式。
type AlbumDTO struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Intro      string    `json:"intro"`
	ImageCount int64     `json:"image_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AlbumInput 是创建或更新相册的输入。
type AlbumInput struct {
	Name  string
	Intro string
}

// AlbumService 管理用户相册及其归属校验。
type AlbumService struct {
	repo *store.Repository
}

// NewAlbumService 构造一个 AlbumService。
func NewAlbumService(repo *store.Repository) *AlbumService {
	return &AlbumService{repo: repo}
}

// List 返回 principal 可见的相册：管理员可见全部，其余人仅可见自己的。
func (s *AlbumService) List(ctx context.Context, principal *auth.Principal) ([]AlbumDTO, error) {
	filter := ""
	if !principal.IsAdmin() {
		filter = principal.UserID
	}
	albums, err := s.repo.ListAlbums(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list albums: %w", err)
	}
	dtos := make([]AlbumDTO, 0, len(albums))
	for i := range albums {
		dtos = append(dtos, *albumToDTO(&albums[i].Album, albums[i].ImageCount))
	}
	return dtos, nil
}

// Create 为 principal 创建一个相册。
func (s *AlbumService) Create(ctx context.Context, principal *auth.Principal, in AlbumInput) (*AlbumDTO, error) {
	name, intro, err := validateAlbumInput(in)
	if err != nil {
		return nil, err
	}
	album := &store.Album{
		ID:     uuid.NewString(),
		UserID: ownerIDOf(principal),
		Name:   name,
		Intro:  intro,
	}
	if err := s.repo.CreateAlbum(ctx, album); err != nil {
		return nil, fmt.Errorf("create album: %w", err)
	}
	return albumToDTO(album, 0), nil
}

// Get 返回单个相册，并强制校验所有权。
func (s *AlbumService) Get(ctx context.Context, principal *auth.Principal, id string) (*AlbumDTO, error) {
	album, err := s.owned(ctx, principal, id)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountImagesInAlbum(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get album: %w", err)
	}
	return albumToDTO(album, count), nil
}

// Update 修改相册的名称与简介，并强制校验所有权。
func (s *AlbumService) Update(ctx context.Context, principal *auth.Principal, id string, in AlbumInput) (*AlbumDTO, error) {
	if _, err := s.owned(ctx, principal, id); err != nil {
		return nil, err
	}
	name, intro, err := validateAlbumInput(in)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateAlbum(ctx, id, name, intro)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, fmt.Errorf("update album: %w", err)
	}
	count, err := s.repo.CountImagesInAlbum(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update album: %w", err)
	}
	return albumToDTO(updated, count), nil
}

// Delete 删除相册，其中的图片会被移出相册但保留。强制校验所有权。
func (s *AlbumService) Delete(ctx context.Context, principal *auth.Principal, id string) error {
	if _, err := s.owned(ctx, principal, id); err != nil {
		return err
	}
	if err := s.repo.DeleteAlbum(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrAlbumNotFound
		}
		return fmt.Errorf("delete album: %w", err)
	}
	return nil
}

// owned 加载相册并校验 principal 是否有权访问。
func (s *AlbumService) owned(ctx context.Context, principal *auth.Principal, id string) (*store.Album, error) {
	album, err := s.repo.GetAlbumByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, fmt.Errorf("get album: %w", err)
	}
	if !canAccess(principal, album.UserID) {
		return nil, ErrForbidden
	}
	return album, nil
}

// validateAlbumInput 规范化并校验相册输入。
func validateAlbumInput(in AlbumInput) (string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > maxAlbumNameRunes {
		return "", "", fmt.Errorf("%w: album name must be 1-%d characters", ErrInvalidInput, maxAlbumNameRunes)
	}
	intro := strings.TrimSpace(in.Intro)
	if len([]rune(intro)) > maxAlbumIntroRunes {
		return "", "", fmt.Errorf("%w: album intro must be at most %d characters", ErrInvalidInput, maxAlbumIntroRunes)
	}
	return name, intro, nil
}

// albumToDTO 组装相册的对外表示。
func albumToDTO(album *store.Album, imageCount int64) *AlbumDTO {
	return &AlbumDTO{
		ID:         album.ID,
		Name:       album.Name,
		Intro:      album.Intro,
		ImageCount: imageCount,
		CreatedAt:  album.CreatedAt,
		UpdatedAt:  album.UpdatedAt,
	}
}
