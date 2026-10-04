package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

type roleGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

type policyRequest struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Settings    json.RawMessage `json:"settings"`
}

type attachPolicyRequest struct {
	PolicyID string `json:"policy_id"`
}

// adminListRoleGroups 列出全部角色组（含策略）。
func (h *Handler) adminListRoleGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.policies.ListRoleGroups(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, groups)
}

// adminCreateRoleGroup 新建角色组。
func (h *Handler) adminCreateRoleGroup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRoleGroupRequest(w, r)
	if !ok {
		return
	}
	group, err := h.policies.CreateRoleGroup(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, group)
}

// adminGetRoleGroup 返回单个角色组。
func (h *Handler) adminGetRoleGroup(w http.ResponseWriter, r *http.Request) {
	group, err := h.policies.GetRoleGroup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, group)
}

// adminUpdateRoleGroup 修改角色组。
func (h *Handler) adminUpdateRoleGroup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRoleGroupRequest(w, r)
	if !ok {
		return
	}
	group, err := h.policies.UpdateRoleGroup(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, group)
}

// adminDeleteRoleGroup 删除角色组。
func (h *Handler) adminDeleteRoleGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.policies.DeleteRoleGroup(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// adminAttachPolicy 把策略绑定到角色组。
func (h *Handler) adminAttachPolicy(w http.ResponseWriter, r *http.Request) {
	var body attachPolicyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	group, err := h.policies.AttachPolicy(r.Context(), chi.URLParam(r, "id"), body.PolicyID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, group)
}

// adminDetachPolicy 解除角色组与策略的绑定。
func (h *Handler) adminDetachPolicy(w http.ResponseWriter, r *http.Request) {
	group, err := h.policies.DetachPolicy(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "policyID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, group)
}

// adminListPolicies 列出策略，可按类型过滤。
func (h *Handler) adminListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.policies.ListPolicies(r.Context(), r.URL.Query().Get("type"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, policies)
}

// adminCreatePolicy 新建策略。
func (h *Handler) adminCreatePolicy(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePolicyRequest(w, r)
	if !ok {
		return
	}
	policy, err := h.policies.CreatePolicy(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, policy)
}

// adminGetPolicy 返回单个策略。
func (h *Handler) adminGetPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := h.policies.GetPolicy(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, policy)
}

// adminUpdatePolicy 修改策略。
func (h *Handler) adminUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePolicyRequest(w, r)
	if !ok {
		return
	}
	policy, err := h.policies.UpdatePolicy(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, policy)
}

// adminDeletePolicy 删除策略。
func (h *Handler) adminDeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.policies.DeletePolicy(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// authPolicies 返回当前主体最终生效的策略集合。
func (h *Handler) authPolicies(w http.ResponseWriter, r *http.Request) {
	effective, err := h.policies.ResolveForCustomer(r.Context(), principalOf(r).UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, effective)
}

func decodeRoleGroupRequest(w http.ResponseWriter, r *http.Request) (service.RoleGroupInput, bool) {
	var body roleGroupRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.RoleGroupInput{}, false
	}
	return service.RoleGroupInput{Name: body.Name, Description: body.Description, IsDefault: body.IsDefault}, true
}

func decodePolicyRequest(w http.ResponseWriter, r *http.Request) (service.PolicyInput, bool) {
	var body policyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.PolicyInput{}, false
	}
	return service.PolicyInput{
		Name:        body.Name,
		Type:        body.Type,
		Description: body.Description,
		Enabled:     body.Enabled,
		Settings:    body.Settings,
	}, true
}
