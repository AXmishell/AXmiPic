package axmipic

import "context"

// Credentials 是登录/注册请求体。
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register 注册一个普通用户。
func (c *Client) Register(ctx context.Context, username, password string) (*User, error) {
	var out User
	if err := c.postJSON(ctx, "/api/v1/auth/register", Credentials{Username: username, Password: password}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Login 以普通用户登录，成功后自动保存令牌。
func (c *Client) Login(ctx context.Context, username, password string) (*Session, error) {
	var out Session
	if err := c.postJSON(ctx, "/api/v1/auth/login", Credentials{Username: username, Password: password}, &out); err != nil {
		return nil, err
	}
	c.token = out.Token
	return &out, nil
}

// LoginAdmin 以管理员登录，成功后自动保存令牌。
func (c *Client) LoginAdmin(ctx context.Context, username, password string) (*Session, error) {
	var out Session
	if err := c.postJSON(ctx, "/api/v1/admin/auth/login", Credentials{Username: username, Password: password}, &out); err != nil {
		return nil, err
	}
	c.token = out.Token
	return &out, nil
}

// Me 返回当前账号信息。
func (c *Client) Me(ctx context.Context) (*User, error) {
	var out User
	if err := c.get(ctx, "/api/v1/auth/me", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ChangePassword 修改当前账户密码（需提供当前密码）。
func (c *Client) ChangePassword(ctx context.Context, currentPassword, newPassword string) error {
	return c.postJSON(ctx, "/api/v1/auth/password", map[string]string{
		"current_password": currentPassword,
		"new_password":     newPassword,
	}, nil)
}

// SendPasswordResetCode 向邮箱发送密码重置验证码（无需登录）。
func (c *Client) SendPasswordResetCode(ctx context.Context, email string) error {
	return c.postJSON(ctx, "/api/v1/auth/password/reset/code", map[string]string{"email": email}, nil)
}

// ResetPassword 校验邮箱验证码并重置密码（无需登录）。
func (c *Client) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	return c.postJSON(ctx, "/api/v1/auth/password/reset", map[string]string{
		"email":        email,
		"code":         code,
		"new_password": newPassword,
	}, nil)
}

// Policies 返回当前账户生效的角色策略。
func (c *Client) Policies(ctx context.Context) (*UploadPolicy, error) {
	var out UploadPolicy
	if err := c.get(ctx, "/api/v1/auth/policies", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- API 令牌 ----

// CreateToken 创建一个 API 令牌（明文仅在返回中出现一次）。
func (c *Client) CreateToken(ctx context.Context, name string) (*Token, error) {
	var out Token
	if err := c.postJSON(ctx, "/api/v1/tokens", map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTokens 返回当前账号的令牌。
func (c *Client) ListTokens(ctx context.Context) ([]Token, error) {
	var out []Token
	if err := c.get(ctx, "/api/v1/tokens", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeToken 吊销一个令牌。
func (c *Client) RevokeToken(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/tokens/"+id, nil)
}
