package api

import (
	"encoding/json"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// xPreviewRequest 是 POST /api/v1/import/x/preview 的请求体。
type xPreviewRequest struct {
	URL string `json:"url"`
}

// previewXImport 解析 X 链接并返回作者与候选图片，不落库。
func (h *Handler) previewXImport(w http.ResponseWriter, r *http.Request) {
	if h.xImport == nil {
		h.fail(w, r, service.ErrXImportUnavailable)
		return
	}
	var body xPreviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	post, err := h.xImport.Preview(r.Context(), body.URL)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, post)
}

// xImportRequest 是 POST /api/v1/import/x 的请求体。
type xImportRequest struct {
	URL string `json:"url"`
	// MediaIDs 为空时导入帖子中的全部图片。
	MediaIDs []string `json:"media_ids"`
	// Permission 为 public（默认）或 private。
	Permission string `json:"permission"`
}

// importX 下载并导入一条 X 帖子中的图片。
func (h *Handler) importX(w http.ResponseWriter, r *http.Request) {
	if h.xImport == nil {
		h.fail(w, r, service.ErrXImportUnavailable)
		return
	}
	var body xImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := h.xImport.Import(r.Context(), principalOf(r), service.XImportInput{
		URL:        body.URL,
		MediaIDs:   body.MediaIDs,
		Permission: body.Permission,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}
