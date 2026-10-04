package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// 站点内容服务返回的错误。
var (
	// ErrAnnouncementNotFound 表示公告不存在。
	ErrAnnouncementNotFound = errors.New("service: announcement not found")
	// ErrReportNotFound 表示举报不存在。
	ErrReportNotFound = errors.New("service: report not found")
	// ErrPageNotFound 表示页面不存在。
	ErrPageNotFound = errors.New("service: page not found")
)

// slugPattern 约束独立页面的 slug 形态。
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)

// AnnouncementDTO 是公告在 API 中的表示形式。
type AnnouncementDTO struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Level     string    `json:"level"`
	Pinned    bool      `json:"pinned"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnnouncementInput 是创建或更新公告的输入。
type AnnouncementInput struct {
	Title     string
	Content   string
	Level     string
	Pinned    bool
	Published bool
}

// ReportDTO 是举报在 API 中的表示形式。
type ReportDTO struct {
	ID           string    `json:"id"`
	ImageID      string    `json:"image_id,omitempty"`
	ReporterID   string    `json:"reporter_id,omitempty"`
	ReporterName string    `json:"reporter_name,omitempty"`
	Reason       string    `json:"reason"`
	Detail       string    `json:"detail"`
	Status       string    `json:"status"`
	HandlerNote  string    `json:"handler_note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ReportInput 是提交举报的输入。
type ReportInput struct {
	ImageID string
	Reason  string
	Detail  string
}

// PageDTO 是独立页面在 API 中的表示形式。
type PageDTO struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PageInput 是创建或更新页面的输入。
type PageInput struct {
	Slug      string
	Title     string
	Content   string
	Published bool
}

// SiteService 管理公告、举报与独立页面。
type SiteService struct {
	repo *store.Repository
}

// NewSiteService 构造一个 SiteService。
func NewSiteService(repo *store.Repository) *SiteService {
	return &SiteService{repo: repo}
}

// ---- 公告 ----

// ListAnnouncements 返回公告；publicOnly 为真时仅返回已发布的。
func (s *SiteService) ListAnnouncements(ctx context.Context, publicOnly bool) ([]AnnouncementDTO, error) {
	items, err := s.repo.ListAnnouncements(ctx, publicOnly)
	if err != nil {
		return nil, fmt.Errorf("list announcements: %w", err)
	}
	dtos := make([]AnnouncementDTO, 0, len(items))
	for i := range items {
		dtos = append(dtos, *announcementToDTO(&items[i]))
	}
	return dtos, nil
}

// CreateAnnouncement 创建公告。
func (s *SiteService) CreateAnnouncement(ctx context.Context, in AnnouncementInput) (*AnnouncementDTO, error) {
	title, content, level, err := validateAnnouncementInput(in)
	if err != nil {
		return nil, err
	}
	item := &store.Announcement{
		ID:        uuid.NewString(),
		Title:     title,
		Content:   content,
		Level:     level,
		Pinned:    in.Pinned,
		Published: in.Published,
	}
	if err := s.repo.CreateAnnouncement(ctx, item); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
	}
	return announcementToDTO(item), nil
}

// UpdateAnnouncement 修改公告。
func (s *SiteService) UpdateAnnouncement(ctx context.Context, id string, in AnnouncementInput) (*AnnouncementDTO, error) {
	title, content, level, err := validateAnnouncementInput(in)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateAnnouncement(ctx, id, store.AnnouncementUpdate{
		Title:     &title,
		Content:   &content,
		Level:     &level,
		Pinned:    &in.Pinned,
		Published: &in.Published,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, fmt.Errorf("update announcement: %w", err)
	}
	return announcementToDTO(updated), nil
}

// DeleteAnnouncement 删除公告。
func (s *SiteService) DeleteAnnouncement(ctx context.Context, id string) error {
	if err := s.repo.DeleteAnnouncement(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrAnnouncementNotFound
		}
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

// ---- 举报 ----

// CreateReport 提交一条举报。当携带图片 id 时会校验其存在。
func (s *SiteService) CreateReport(ctx context.Context, principal *auth.Principal, in ReportInput) (*ReportDTO, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len([]rune(reason)) > 100 {
		return nil, fmt.Errorf("%w: reason must be 1-100 characters", ErrInvalidInput)
	}
	detail := strings.TrimSpace(in.Detail)
	if len([]rune(detail)) > 1000 {
		return nil, fmt.Errorf("%w: detail must be at most 1000 characters", ErrInvalidInput)
	}
	var imageID *string
	if id := strings.TrimSpace(in.ImageID); id != "" {
		if _, err := s.repo.GetByID(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("create report: %w", err)
		}
		imageID = &id
	}
	report := &store.Report{
		ID:      uuid.NewString(),
		ImageID: imageID,
		Reason:  reason,
		Detail:  detail,
		Status:  store.ReportPending,
	}
	if principal != nil && !principal.IsGuest() {
		reporter := principal.UserID
		report.Reporter = &reporter
	}
	if err := s.repo.CreateReport(ctx, report); err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}
	return s.reportToDTO(ctx, report), nil
}

// ListReports 返回举报；status 非空时按状态过滤。
func (s *SiteService) ListReports(ctx context.Context, status string) ([]ReportDTO, error) {
	if status != "" && !isValidReportStatus(status) {
		return nil, fmt.Errorf("%w: unknown report status %q", ErrInvalidInput, status)
	}
	reports, err := s.repo.ListReports(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	// 批量解析举报人用户名，避免 N+1。
	ids := make([]string, 0, len(reports))
	seen := make(map[string]struct{})
	for i := range reports {
		if reports[i].Reporter == nil {
			continue
		}
		id := *reports[i].Reporter
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		if resolved, err := s.repo.UsernamesByIDs(ctx, ids); err == nil {
			names = resolved
		}
	}
	dtos := make([]ReportDTO, 0, len(reports))
	for i := range reports {
		dto := reportToDTO(&reports[i])
		if reports[i].Reporter != nil {
			dto.ReporterName = names[*reports[i].Reporter]
		}
		dtos = append(dtos, *dto)
	}
	return dtos, nil
}

// UpdateReportStatus 更新举报状态与处理备注。
func (s *SiteService) UpdateReportStatus(ctx context.Context, id, status, note string) (*ReportDTO, error) {
	status = strings.TrimSpace(status)
	if !isValidReportStatus(status) {
		return nil, fmt.Errorf("%w: unknown report status %q", ErrInvalidInput, status)
	}
	if len([]rune(note)) > 1000 {
		return nil, fmt.Errorf("%w: handler note must be at most 1000 characters", ErrInvalidInput)
	}
	updated, err := s.repo.UpdateReportStatus(ctx, id, status, strings.TrimSpace(note))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrReportNotFound
		}
		return nil, fmt.Errorf("update report: %w", err)
	}
	return s.reportToDTO(ctx, updated), nil
}

// ---- 页面 ----

// ListPages 返回页面；publicOnly 为真时仅返回已发布的。
func (s *SiteService) ListPages(ctx context.Context, publicOnly bool) ([]PageDTO, error) {
	pages, err := s.repo.ListPages(ctx, publicOnly)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	dtos := make([]PageDTO, 0, len(pages))
	for i := range pages {
		dtos = append(dtos, *pageToDTO(&pages[i]))
	}
	return dtos, nil
}

// GetPageBySlug 返回一个已发布的页面。
func (s *SiteService) GetPageBySlug(ctx context.Context, slug string) (*PageDTO, error) {
	page, err := s.repo.GetPageBySlug(ctx, strings.TrimSpace(slug))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPageNotFound
		}
		return nil, fmt.Errorf("get page: %w", err)
	}
	if !page.Published {
		return nil, ErrPageNotFound
	}
	return pageToDTO(page), nil
}

// CreatePage 创建独立页面。
func (s *SiteService) CreatePage(ctx context.Context, in PageInput) (*PageDTO, error) {
	slug, title, content, err := validatePageInput(in)
	if err != nil {
		return nil, err
	}
	page := &store.Page{
		ID:        uuid.NewString(),
		Slug:      slug,
		Title:     title,
		Content:   content,
		Published: in.Published,
	}
	if err := s.repo.CreatePage(ctx, page); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: slug already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create page: %w", err)
	}
	return pageToDTO(page), nil
}

// UpdatePage 修改独立页面。
func (s *SiteService) UpdatePage(ctx context.Context, id string, in PageInput) (*PageDTO, error) {
	slug, title, content, err := validatePageInput(in)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdatePage(ctx, id, store.PageUpdate{
		Slug:      &slug,
		Title:     &title,
		Content:   &content,
		Published: &in.Published,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPageNotFound
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: slug already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("update page: %w", err)
	}
	return pageToDTO(updated), nil
}

// DeletePage 删除独立页面。
func (s *SiteService) DeletePage(ctx context.Context, id string) error {
	if err := s.repo.DeletePage(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrPageNotFound
		}
		return fmt.Errorf("delete page: %w", err)
	}
	return nil
}

// reportToDTO 组装举报的对外表示（不含举报人用户名）。
func (s *SiteService) reportToDTO(ctx context.Context, report *store.Report) *ReportDTO {
	dto := reportToDTO(report)
	if report.Reporter != nil {
		if names, err := s.repo.UsernamesByIDs(ctx, []string{*report.Reporter}); err == nil {
			dto.ReporterName = names[*report.Reporter]
		}
	}
	return dto
}

func validateAnnouncementInput(in AnnouncementInput) (string, string, string, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len([]rune(title)) > 200 {
		return "", "", "", fmt.Errorf("%w: title must be 1-200 characters", ErrInvalidInput)
	}
	content := strings.TrimSpace(in.Content)
	if len([]rune(content)) > 20000 {
		return "", "", "", fmt.Errorf("%w: content is too long", ErrInvalidInput)
	}
	level := strings.TrimSpace(in.Level)
	switch level {
	case "", "info":
		level = "info"
	case "success", "warning", "danger":
	default:
		return "", "", "", fmt.Errorf("%w: unknown level %q", ErrInvalidInput, in.Level)
	}
	return title, content, level, nil
}

func validatePageInput(in PageInput) (string, string, string, error) {
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugPattern.MatchString(slug) {
		return "", "", "", fmt.Errorf("%w: slug must match [a-z0-9-], starting with a letter or digit", ErrInvalidInput)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len([]rune(title)) > 200 {
		return "", "", "", fmt.Errorf("%w: title must be 1-200 characters", ErrInvalidInput)
	}
	content := strings.TrimSpace(in.Content)
	if len([]rune(content)) > 100000 {
		return "", "", "", fmt.Errorf("%w: content is too long", ErrInvalidInput)
	}
	return slug, title, content, nil
}

func isValidReportStatus(status string) bool {
	switch status {
	case store.ReportPending, store.ReportResolved, store.ReportRejected:
		return true
	default:
		return false
	}
}

func announcementToDTO(a *store.Announcement) *AnnouncementDTO {
	return &AnnouncementDTO{
		ID:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Level:     a.Level,
		Pinned:    a.Pinned,
		Published: a.Published,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

func reportToDTO(r *store.Report) *ReportDTO {
	imageID := ""
	if r.ImageID != nil {
		imageID = *r.ImageID
	}
	reporterID := ""
	if r.Reporter != nil {
		reporterID = *r.Reporter
	}
	return &ReportDTO{
		ID:          r.ID,
		ImageID:     imageID,
		ReporterID:  reporterID,
		Reason:      r.Reason,
		Detail:      r.Detail,
		Status:      r.Status,
		HandlerNote: r.HandlerNote,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func pageToDTO(p *store.Page) *PageDTO {
	return &PageDTO{
		ID:        p.ID,
		Slug:      p.Slug,
		Title:     p.Title,
		Content:   p.Content,
		Published: p.Published,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
