package service

import (
	"context"
	"fmt"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// ImageUpdate 是图片元数据的一次部分更新；nil 表示不修改该字段。
type ImageUpdate struct {
	OriginalName *string
	AuthorName   *string
	AuthorHandle *string
	SourceURL    *string
}

// HasMetadata 报告本次更新是否包含作者/来源字段（这些字段仅管理员可改）。
func (u ImageUpdate) HasMetadata() bool {
	return u.AuthorName != nil || u.AuthorHandle != nil || u.SourceURL != nil
}

// Empty 报告本次更新是否没有提供任何字段。
func (u ImageUpdate) Empty() bool {
	return u.OriginalName == nil && !u.HasMetadata()
}

// UpdateImage 更新图片展示名或作者/来源信息。展示名允许所有者或管理员修改；
// 作者/来源字段只允许管理员修改，避免普通用户冒用署名。
func (s *UploadService) UpdateImage(ctx context.Context, principal *auth.Principal, id string, update ImageUpdate) (*ImageDTO, error) {
	if update.Empty() {
		return nil, fmt.Errorf("%w: no changes requested", ErrInvalidInput)
	}
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return nil, ErrForbidden
	}
	if update.HasMetadata() && !principal.IsAdmin() {
		return nil, fmt.Errorf("%w: only administrators can edit author or source", ErrForbidden)
	}

	patch := store.ImagePatch{}
	if update.OriginalName != nil {
		name := sanitizeOriginalName(*update.OriginalName)
		if name == "" {
			return nil, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
		}
		patch.OriginalName = &name
	}
	if update.HasMetadata() {
		meta := ImageMeta{
			AuthorName:     image.AuthorName,
			AuthorHandle:   image.AuthorHandle,
			AuthorPlatform: image.AuthorPlatform,
			SourceURL:      image.SourceURL,
			SourcePostID:   image.SourcePostID,
		}
		if update.AuthorName != nil {
			meta.AuthorName = *update.AuthorName
		}
		if update.AuthorHandle != nil {
			meta.AuthorHandle = *update.AuthorHandle
		}
		if update.SourceURL != nil {
			meta.SourceURL = *update.SourceURL
		}
		normalized, err := NormalizeImageMeta(meta)
		if err != nil {
			return nil, err
		}
		patch.AuthorName = &normalized.AuthorName
		patch.AuthorHandle = &normalized.AuthorHandle
		patch.AuthorPlatform = &normalized.AuthorPlatform
		patch.SourceURL = &normalized.SourceURL
		patch.SourcePostID = &normalized.SourcePostID
	}

	updated, err := s.repo.UpdateImage(ctx, id, patch)
	if err != nil {
		return nil, fmt.Errorf("update image: %w", err)
	}
	return toDTO(updated, s.mediaBaseURL), nil
}
