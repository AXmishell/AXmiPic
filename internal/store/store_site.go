package store

import (
	"context"
	"fmt"
)

// CreateAnnouncement 插入一条公告。
func (r *Repository) CreateAnnouncement(ctx context.Context, a *Announcement) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("store: create announcement: %w", err)
	}
	return nil
}

// GetAnnouncementByID 返回具有给定 id 的公告，或 ErrNotFound。
func (r *Repository) GetAnnouncementByID(ctx context.Context, id string) (*Announcement, error) {
	var a Announcement
	if err := r.first(ctx, &a, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: announcement %q: %w", id, err)
	}
	return &a, nil
}

// ListAnnouncements 返回公告。publishedOnly 为真时仅返回已发布的，否则返回
// 全部（供管理端使用）。置顶优先，其后按创建时间倒序。
func (r *Repository) ListAnnouncements(ctx context.Context, publishedOnly bool) ([]Announcement, error) {
	query := r.db.WithContext(ctx).Order("pinned DESC, created_at DESC")
	if publishedOnly {
		query = query.Where("published = ?", true)
	}
	var items []Announcement
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("store: list announcements: %w", err)
	}
	return items, nil
}

// AnnouncementUpdate 携带可选的公告字段更改。
type AnnouncementUpdate struct {
	Title     *string
	Content   *string
	Level     *string
	Pinned    *bool
	Published *bool
}

// UpdateAnnouncement 应用可选字段更改并返回更新后的记录。
func (r *Repository) UpdateAnnouncement(ctx context.Context, id string, update AnnouncementUpdate) (*Announcement, error) {
	fields := map[string]any{}
	if update.Title != nil {
		fields["title"] = *update.Title
	}
	if update.Content != nil {
		fields["content"] = *update.Content
	}
	if update.Level != nil {
		fields["level"] = *update.Level
	}
	if update.Pinned != nil {
		fields["pinned"] = *update.Pinned
	}
	if update.Published != nil {
		fields["published"] = *update.Published
	}
	if len(fields) > 0 {
		if err := r.db.WithContext(ctx).Model(&Announcement{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return nil, fmt.Errorf("store: update announcement %q: %w", id, err)
		}
	}
	return r.GetAnnouncementByID(ctx, id)
}

// DeleteAnnouncement 移除一条公告。
func (r *Repository) DeleteAnnouncement(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Announcement{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete announcement %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: announcement %q: %w", id, ErrNotFound)
	}
	return nil
}

// CreateReport 插入一条举报。
func (r *Repository) CreateReport(ctx context.Context, report *Report) error {
	if err := r.db.WithContext(ctx).Create(report).Error; err != nil {
		return fmt.Errorf("store: create report: %w", err)
	}
	return nil
}

// GetReportByID 返回具有给定 id 的举报，或 ErrNotFound。
func (r *Repository) GetReportByID(ctx context.Context, id string) (*Report, error) {
	var report Report
	if err := r.first(ctx, &report, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: report %q: %w", id, err)
	}
	return &report, nil
}

// ListReports 返回举报，可按状态过滤，最新的在前。
func (r *Repository) ListReports(ctx context.Context, status string) ([]Report, error) {
	query := r.db.WithContext(ctx).Order("created_at DESC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var reports []Report
	if err := query.Find(&reports).Error; err != nil {
		return nil, fmt.Errorf("store: list reports: %w", err)
	}
	return reports, nil
}

// UpdateReportStatus 更新举报状态与处理备注。
func (r *Repository) UpdateReportStatus(ctx context.Context, id, status, note string) (*Report, error) {
	result := r.db.WithContext(ctx).Model(&Report{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "handler_note": note})
	if result.Error != nil {
		return nil, fmt.Errorf("store: update report %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("store: report %q: %w", id, ErrNotFound)
	}
	return r.GetReportByID(ctx, id)
}

// CreatePage 插入一个独立页面。
func (r *Repository) CreatePage(ctx context.Context, page *Page) error {
	if err := r.db.WithContext(ctx).Create(page).Error; err != nil {
		return fmt.Errorf("store: create page: %w", err)
	}
	return nil
}

// GetPageByID 返回具有给定 id 的页面，或 ErrNotFound。
func (r *Repository) GetPageByID(ctx context.Context, id string) (*Page, error) {
	var page Page
	if err := r.first(ctx, &page, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: page %q: %w", id, err)
	}
	return &page, nil
}

// GetPageBySlug 返回具有给定 slug 的页面，或 ErrNotFound。
func (r *Repository) GetPageBySlug(ctx context.Context, slug string) (*Page, error) {
	var page Page
	if err := r.first(ctx, &page, "slug = ?", slug); err != nil {
		return nil, fmt.Errorf("store: page slug %q: %w", slug, err)
	}
	return &page, nil
}

// ListPages 返回页面；publishedOnly 为真时仅返回已发布的。
func (r *Repository) ListPages(ctx context.Context, publishedOnly bool) ([]Page, error) {
	query := r.db.WithContext(ctx).Order("created_at ASC")
	if publishedOnly {
		query = query.Where("published = ?", true)
	}
	var pages []Page
	if err := query.Find(&pages).Error; err != nil {
		return nil, fmt.Errorf("store: list pages: %w", err)
	}
	return pages, nil
}

// PageUpdate 携带可选的页面字段更改。
type PageUpdate struct {
	Slug      *string
	Title     *string
	Content   *string
	Published *bool
}

// UpdatePage 应用可选字段更改并返回更新后的记录。
func (r *Repository) UpdatePage(ctx context.Context, id string, update PageUpdate) (*Page, error) {
	fields := map[string]any{}
	if update.Slug != nil {
		fields["slug"] = *update.Slug
	}
	if update.Title != nil {
		fields["title"] = *update.Title
	}
	if update.Content != nil {
		fields["content"] = *update.Content
	}
	if update.Published != nil {
		fields["published"] = *update.Published
	}
	if len(fields) > 0 {
		if err := r.db.WithContext(ctx).Model(&Page{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return nil, fmt.Errorf("store: update page %q: %w", id, err)
		}
	}
	return r.GetPageByID(ctx, id)
}

// DeletePage 移除一个页面。
func (r *Repository) DeletePage(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Page{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete page %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: page %q: %w", id, ErrNotFound)
	}
	return nil
}
