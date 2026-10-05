package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
)

type createShareRequest struct {
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Password       string `json:"password"`
	ExpiresInHours int    `json:"expires_in_hours"`
	MaxViews       int64  `json:"max_views"`
}

type shareAccessRequest struct {
	Password string `json:"password"`
}

// requireFeature 在角色策略中校验某个功能开关是否启用。管理员不受功能开关
// 限制；访客按 Guest 角色组解析，其余按账户所属角色组解析。
func (h *Handler) requireFeature(r *http.Request, principal *auth.Principal, name string) error {
	if h.policies == nil || principal == nil || principal.IsAdmin() {
		return nil
	}
	var (
		effective *service.EffectivePolicies
		err       error
	)
	if principal.IsGuest() {
		effective, err = h.policies.ResolveGuest(r.Context())
	} else {
		effective, err = h.policies.ResolveForCustomer(r.Context(), principal.UserID)
	}
	if err != nil {
		return err
	}
	if !effective.HasFeature(name) {
		return fmt.Errorf("%w: feature %q is not enabled for your role", service.ErrForbidden, name)
	}
	return nil
}

// createShare 为图片或相册创建一个分享链接。
func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	var body createShareRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureShare); err != nil {
		h.fail(w, r, err)
		return
	}
	if body.Password != "" {
		if err := h.requireFeature(r, principal, service.FeatureSharePassword); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	share, err := h.shares.Create(r.Context(), principal, service.ShareInput{
		TargetType:     body.TargetType,
		TargetID:       body.TargetID,
		Password:       body.Password,
		ExpiresInHours: body.ExpiresInHours,
		MaxViews:       body.MaxViews,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, share)
}

// listShares 列出当前主体创建的分享。
func (h *Handler) listShares(w http.ResponseWriter, r *http.Request) {
	shares, err := h.shares.List(r.Context(), principalOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, shares)
}

// revokeShare 删除一个分享。
func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.shares.Revoke(r.Context(), principalOf(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// shareInfo 返回分享的公开元信息。
func (h *Handler) shareInfo(w http.ResponseWriter, r *http.Request) {
	info, err := h.shares.Info(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, info)
}

// shareAccess 校验密码后返回分享内容。
func (h *Handler) shareAccess(w http.ResponseWriter, r *http.Request) {
	var body shareAccessRequest
	// 允许空请求体（无密码分享）。
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body)
	}
	payload, err := h.shares.Access(r.Context(), chi.URLParam(r, "token"), body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, payload)
}
