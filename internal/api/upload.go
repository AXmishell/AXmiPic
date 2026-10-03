package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// multipartOverhead is extra request-body allowance for multipart boundaries
// and headers beyond the file size limit.
const multipartOverhead = 1 << 20

// maxJSONBody bounds the size of JSON request bodies.
const maxJSONBody = 1 << 20

func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request) {
	// Reject an obviously oversized body up front so the client receives a
	// clean 413 instead of having the connection closed mid-upload.
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

// presignRequest is the JSON body for POST /api/v1/upload/presign.
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

// confirmRequest is the JSON body for POST /api/v1/upload/confirm.
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
