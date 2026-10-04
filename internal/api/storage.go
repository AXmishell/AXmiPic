package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

// storageList 返回所有已配置的存储后端。
func (h *Handler) storageList(w http.ResponseWriter, r *http.Request) {
	list, err := h.storageSvc.List(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, list)
}

// storageGet 返回单个存储后端。
func (h *Handler) storageGet(w http.ResponseWriter, r *http.Request) {
	list, err := h.storageSvc.List(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	for i := range list {
		if list[i].ID == id {
			writeOK(w, list[i])
			return
		}
	}
	h.fail(w, r, service.ErrStorageNotFound)
}

type storageRequestBody struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver"`
	Settings map[string]any    `json:"settings"`
	Secrets  map[string]string `json:"secrets"`
	Activate bool              `json:"activate"`
}

// storageCreate 新增一个存储后端。
func (h *Handler) storageCreate(w http.ResponseWriter, r *http.Request) {
	var body storageRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dto, err := h.storageSvc.Create(r.Context(), service.StorageInput{
		Name:     body.Name,
		Driver:   body.Driver,
		Settings: body.Settings,
		Secrets:  body.Secrets,
	}, body.Activate)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, dto)
}

// storageUpdate 修改一个存储后端。
func (h *Handler) storageUpdate(w http.ResponseWriter, r *http.Request) {
	var body storageRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dto, err := h.storageSvc.Update(r.Context(), chi.URLParam(r, "id"), service.StorageInput{
		Name:     body.Name,
		Driver:   body.Driver,
		Settings: body.Settings,
		Secrets:  body.Secrets,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

// storageDelete 删除一个存储后端。
func (h *Handler) storageDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.storageSvc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// storageActivate 将指定后端设为当前默认（热切换）。
func (h *Handler) storageActivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.storageSvc.SetCurrent(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}
