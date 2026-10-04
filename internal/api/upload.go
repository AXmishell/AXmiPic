package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// multipartOverhead 是在文件大小限制之外，为 multipart 边界和头部预留的
// 额外请求体余量。
const multipartOverhead = 1 << 20

// maxJSONBody 限制 JSON 请求体的大小。
const maxJSONBody = 1 << 20

func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	// 预先拒绝明显过大的请求体，这样客户端会收到干净的 413，
	// 而不是在上传中途连接被关闭。
	if r.ContentLength > h.maxUploadBytes+multipartOverhead {
		h.fail(w, r, service.ErrFileTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+multipartOverhead)

	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.fail(w, r, service.ErrFileTooLarge)
			return
		}
		h.logger.WarnContext(r.Context(), "invalid multipart upload", slog.Any("error", err))
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "missing multipart field \"file\"")
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			h.logger.WarnContext(r.Context(), "close uploaded file", slog.Any("error", closeErr))
		}
	}()

	data, err := io.ReadAll(io.LimitReader(file, h.maxUploadBytes+1))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.fail(w, r, service.ErrFileTooLarge)
			return
		}
		h.logger.ErrorContext(r.Context(), "read uploaded file", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, http.StatusInternalServerError, "internal server error")
		return
	}
	if int64(len(data)) > h.maxUploadBytes {
		h.fail(w, r, service.ErrFileTooLarge)
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	mimeType := http.DetectContentType(data)
	dto, err := h.svc.Upload(r.Context(), principalOf(r), service.UploadInput{Data: data, MimeType: mimeType})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
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
	dto, err := h.svc.Confirm(r.Context(), principalOf(r), body.Key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}
