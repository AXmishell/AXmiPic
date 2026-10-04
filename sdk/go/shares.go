package axmipic

import "context"

// ShareInput 是创建分享的输入。
type ShareInput struct {
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Password       string `json:"password,omitempty"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
	MaxViews       int64  `json:"max_views,omitempty"`
}

// CreateShare 为图片或相册创建分享链接。
func (c *Client) CreateShare(ctx context.Context, in ShareInput) (*Share, error) {
	var out Share
	if err := c.postJSON(ctx, "/api/v1/shares", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListShares 返回当前账号创建的分享。
func (c *Client) ListShares(ctx context.Context) ([]Share, error) {
	var out []Share
	if err := c.get(ctx, "/api/v1/shares", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeShare 撤销一个分享。
func (c *Client) RevokeShare(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/shares/"+id, nil)
}

// ShareInfo 返回分享的公开元信息（无需登录）。
func (c *Client) ShareInfo(ctx context.Context, token string) (*Share, error) {
	var out Share
	if err := c.get(ctx, "/api/v1/shares/"+token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccessShare 校验密码并返回分享内容（无需登录）。
func (c *Client) AccessShare(ctx context.Context, token, password string) (*SharePayload, error) {
	var out SharePayload
	if err := c.postJSON(ctx, "/api/v1/shares/"+token+"/access", map[string]string{"password": password}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
