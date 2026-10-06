package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// multipartOverhead 是在文件大小限制之外，为 multipart 边界和头部预留的
// 额外请求体余量。
const multipartOverhead = 1 << 20

// multipartMemoryLimit 是 multipart 解析时驻留内存的上限；超过该值的文件部分
// 会落到临时文件，避免大图上传在内存中留下整份副本。
const multipartMemoryLimit = 1 << 20

// maxJSONBody 限制 JSON 请求体的大小。
const maxJSONBody = 1 << 20

func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	maxBytes := h.currentMaxUploadBytes()
	// 预先拒绝明显过大的请求体，这样客户端会收到干净的 413，
	// 而不是在上传中途连接被关闭。
	if r.ContentLength > maxBytes+multipartOverhead {
		h.fail(w, r, service.ErrFileTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverhead)

	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.fail(w, r, service.ErrFileTooLarge)
			return
		}
		h.logger.WarnContext(r.Context(), "invalid multipart upload", slog.Any("error", err))
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "missing multipart field \"file\"")
		return
	}
	originalName := ""
	size := int64(-1)
	if header != nil {
		originalName = header.Filename
		size = header.Size
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			h.logger.WarnContext(r.Context(), "close uploaded file", slog.Any("error", closeErr))
		}
	}()

	if size <= 0 {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "uploaded file is empty")
		return
	}
	if size > maxBytes {
		h.fail(w, r, service.ErrFileTooLarge)
		return
	}

	principal := principalOf(r)
	ctx := r.Context()
	if principal.IsGuest() {
		ctx = service.WithClientIP(ctx, requestClientIP(r))
	}
	// 直接流式读取 multipart 文件，mimeType 由服务层按有界前缀嗅探。
	dto, err := h.svc.UploadStream(ctx, principal, file, size, "", originalName)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

// currentMaxUploadBytes 返回当前生效的单文件上传上限（字节）：优先读取可在
// 后台热更新的上传设置，未配置设置服务时回退到启动时的配置值。
func (h *Handler) currentMaxUploadBytes() int64 {
	if h.settings != nil {
		if v, ok := service.DomainValue[service.UploadSettings](h.settings, "upload"); ok && v.MaxSizeMB > 0 {
			return int64(v.MaxSizeMB) << 20
		}
	}
	return h.maxUploadBytes
}

// presignRequest 是 POST /api/v1/upload/presign 的 JSON 请求体。
type presignRequest struct {
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

func (h *Handler) presignUpload(w http.ResponseWriter, r *http.Request) {
	var body presignRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := h.svc.Presign(r.Context(), principalOf(r), service.PresignInput{MimeType: body.MimeType, Size: body.Size})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

// confirmRequest 是 POST /api/v1/upload/confirm 的 JSON 请求体。
type confirmRequest struct {
	Key string `json:"key"`
}

func (h *Handler) confirmUpload(w http.ResponseWriter, r *http.Request) {
	var body confirmRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	ctx := r.Context()
	if principal.IsGuest() {
		ctx = service.WithClientIP(ctx, requestClientIP(r))
	}
	dto, err := h.svc.Confirm(ctx, principal, body.Key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}
