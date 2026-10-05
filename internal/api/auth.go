package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// registerRequest 是注册请求体：邮箱需先通过验证码验证。
type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Code     string `json:"code"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.RegisterCustomerWithEmail(r.Context(), body.Username, body.Password, body.Email, body.Code)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, user)
}

// sendRegisterCode 向邮箱发送注册验证码（无需登录）。
func (h *Handler) sendRegisterCode(w http.ResponseWriter, r *http.Request) {
	var body emailRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.SendRegistrationCode(r.Context(), body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "sent"})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session, err := h.accounts.LoginCustomer(r.Context(), body.Username, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, session)
}

func (h *Handler) adminLogin(w http.ResponseWriter, r *http.Request) {
	var body credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session, err := h.accounts.LoginAdmin(r.Context(), body.Username, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, session)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	if principal == nil || principal.UserID == "" {
		writeError(w, http.StatusUnauthorized, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := h.accounts.Me(r.Context(), principal)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

type createTokenRequest struct {
	Name string `json:"name"`
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var body createTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureAPITokens); err != nil {
		h.fail(w, r, err)
		return
	}
	token, err := h.accounts.CreateToken(r.Context(), principal.UserID, body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, token)
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	tokens, err := h.accounts.ListTokens(r.Context(), principal.UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, tokens)
}

func (h *Handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	id := chi.URLParam(r, "id")
	if err := h.accounts.RevokeToken(r.Context(), principal.UserID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// ---- 二次验证（TOTP）与邮箱绑定 ----

type totpVerifyRequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}

// verifyTOTPLogin 完成登录时的 TOTP 二次验证。
func (h *Handler) verifyTOTPLogin(w http.ResponseWriter, r *http.Request) {
	var body totpVerifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session, err := h.accounts.VerifyTOTPLogin(r.Context(), body.ChallengeToken, body.Code)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, session)
}

// securityInfo 返回当前账户的安全设置状态。
func (h *Handler) securityInfo(w http.ResponseWriter, r *http.Request) {
	info, err := h.accounts.Security(r.Context(), principalOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, info)
}

// setupTOTP 生成 TOTP 密钥并返回 otpauth 链接。
func (h *Handler) setupTOTP(w http.ResponseWriter, r *http.Request) {
	setup, err := h.accounts.SetupTOTP(r.Context(), principalOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, setup)
}

type totpCodeRequest struct {
	Code string `json:"code"`
}

// enableTOTP 校验动态码后启用二次验证。
func (h *Handler) enableTOTP(w http.ResponseWriter, r *http.Request) {
	var body totpCodeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.EnableTOTP(r.Context(), principalOf(r), body.Code)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

type totpDisableRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

// disableTOTP 关闭二次验证（需动态码或密码）。
func (h *Handler) disableTOTP(w http.ResponseWriter, r *http.Request) {
	var body totpDisableRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.DisableTOTP(r.Context(), principalOf(r), body.Code, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

type emailRequest struct {
	Email string `json:"email"`
}

// sendEmailCode 向目标邮箱发送验证码。
func (h *Handler) sendEmailCode(w http.ResponseWriter, r *http.Request) {
	var body emailRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.SendEmailVerification(r.Context(), principalOf(r), body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "sent"})
}

type emailVerifyRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

// verifyEmail 校验验证码并绑定邮箱；换绑已绑定的邮箱时需提供当前密码。
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body emailVerifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.VerifyEmail(r.Context(), principalOf(r), body.Email, body.Code, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

// unbindEmail 解绑邮箱（需当前密码）。
func (h *Handler) unbindEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.UnbindEmail(r.Context(), principalOf(r), body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

// ---- 修改密码与找回密码 ----

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword 在验证当前密码后修改当前账户密码。
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var body changePasswordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.ChangePassword(r.Context(), principalOf(r), body.CurrentPassword, body.NewPassword); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "updated"})
}

// sendPasswordResetCode 向已验证邮箱发送密码重置验证码（无需登录）。
func (h *Handler) sendPasswordResetCode(w http.ResponseWriter, r *http.Request) {
	var body emailRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.SendPasswordResetCode(r.Context(), body.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	// 始终返回成功，避免暴露邮箱是否已注册。
	writeOK(w, map[string]string{"status": "sent"})
}

type passwordResetRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

// resetPassword 校验邮箱验证码并重置密码（无需登录）。
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body passwordResetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.ResetPassword(r.Context(), body.Email, body.Code, body.NewPassword); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "updated"})
}
