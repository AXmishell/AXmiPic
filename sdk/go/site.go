package axmipic

import "context"

// AnnouncementInput 是创建或更新公告的输入。
type AnnouncementInput struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Level     string `json:"level"`
	Pinned    bool   `json:"pinned"`
	Published bool   `json:"published"`
}

// PageInput 是创建或更新页面的输入。
type PageInput struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Published bool   `json:"published"`
}

// ReportInput 是提交举报的输入。
type ReportInput struct {
	ImageID string `json:"image_id,omitempty"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
}

// ---- 公开接口 ----

// ListAnnouncements 返回已发布的公告（无需登录）。
func (c *Client) ListAnnouncements(ctx context.Context) ([]Announcement, error) {
	var out []Announcement
	if err := c.get(ctx, "/api/v1/announcements", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPage 返回已发布的独立页面（无需登录）。
func (c *Client) GetPage(ctx context.Context, slug string) (*Page, error) {
	var out Page
	if err := c.get(ctx, "/api/v1/pages/"+slug, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateReport 提交一条举报。
func (c *Client) CreateReport(ctx context.Context, in ReportInput) (*Report, error) {
	var out Report
	if err := c.postJSON(ctx, "/api/v1/reports", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- 管理接口 ----

// AdminListAnnouncements 返回全部公告。
func (c *Client) AdminListAnnouncements(ctx context.Context) ([]Announcement, error) {
	var out []Announcement
	if err := c.get(ctx, "/api/v1/admin/announcements", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreateAnnouncement 创建公告。
func (c *Client) AdminCreateAnnouncement(ctx context.Context, in AnnouncementInput) (*Announcement, error) {
	var out Announcement
	if err := c.postJSON(ctx, "/api/v1/admin/announcements", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateAnnouncement 修改公告。
func (c *Client) AdminUpdateAnnouncement(ctx context.Context, id string, in AnnouncementInput) (*Announcement, error) {
	var out Announcement
	if err := c.putJSON(ctx, "/api/v1/admin/announcements/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteAnnouncement 删除公告。
func (c *Client) AdminDeleteAnnouncement(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/announcements/"+id, nil)
}

// AdminListReports 返回举报，可按状态过滤。
func (c *Client) AdminListReports(ctx context.Context, status string) ([]Report, error) {
	var out []Report
	path := encodeQuery("/api/v1/admin/reports", map[string]string{"status": status})
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminUpdateReport 处理举报。
func (c *Client) AdminUpdateReport(ctx context.Context, id, status, note string) (*Report, error) {
	var out Report
	in := map[string]string{"status": status, "note": note}
	if err := c.patchJSON(ctx, "/api/v1/admin/reports/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminListPages 返回全部独立页面。
func (c *Client) AdminListPages(ctx context.Context) ([]Page, error) {
	var out []Page
	if err := c.get(ctx, "/api/v1/admin/pages", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreatePage 创建独立页面。
func (c *Client) AdminCreatePage(ctx context.Context, in PageInput) (*Page, error) {
	var out Page
	if err := c.postJSON(ctx, "/api/v1/admin/pages", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdatePage 修改独立页面。
func (c *Client) AdminUpdatePage(ctx context.Context, id string, in PageInput) (*Page, error) {
	var out Page
	if err := c.putJSON(ctx, "/api/v1/admin/pages/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeletePage 删除独立页面。
func (c *Client) AdminDeletePage(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/pages/"+id, nil)
}
