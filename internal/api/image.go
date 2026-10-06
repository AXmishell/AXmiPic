package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "ok", Data: map[string]string{"status": "ok"}})
}

func (h *Handler) listImages(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.List(
		r.Context(),
		principalOf(r),
		queryInt(r, "page"),
		queryInt(r, "page_size"),
		imageFilterFromQuery(r),
	)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

// listPlaza 返回公开图片广场的一页图片。
func (h *Handler) listPlaza(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.ListPlaza(
		r.Context(),
		queryInt(r, "page"),
		queryInt(r, "page_size"),
		imageFilterFromQuery(r),
	)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

// imageFilterFromQuery 从查询参数构建图片过滤条件。
func imageFilterFromQuery(r *http.Request) service.ImageFilter {
	query := r.URL.Query()
	filter := service.ImageFilter{
		Order:      query.Get("order"),
		Keyword:    query.Get("keyword"),
		Permission: query.Get("permission"),
		UserID:     query.Get("user_id"),
		Cursor:     strings.TrimSpace(query.Get("cursor")),
	}
	if album := strings.TrimSpace(query.Get("album_id")); album != "" {
		filter.AlbumID = &album
	}
	return filter
}

func (h *Handler) getImage(w http.ResponseWriter, r *http.Request) {
	dto, err := h.svc.Get(r.Context(), principalOf(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

// renameImageRequest 是 PATCH /api/v1/images/{id} 的 JSON 请求体。
type renameImageRequest struct {
	Name string `json:"name"`
}

func (h *Handler) renameImage(w http.ResponseWriter, r *http.Request) {
	var body renameImageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dto, err := h.svc.Rename(r.Context(), principalOf(r), chi.URLParam(r, "id"), body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), principalOf(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// batchImagesRequest 是 POST /api/v1/images/batch 的请求体。permission 与
// album_id / clear_album 至少提供其一。
type batchImagesRequest struct {
	IDs        []string `json:"ids"`
	Permission *string  `json:"permission"`
	AlbumID    *string  `json:"album_id"`
	ClearAlbum bool     `json:"clear_album"`
}

// batchImages 对一组图片批量设置可见性或所属相册。
func (h *Handler) batchImages(w http.ResponseWriter, r *http.Request) {
	var body batchImagesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Permission == nil && body.AlbumID == nil && !body.ClearAlbum {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "no changes requested")
		return
	}
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureBatchUpload); err != nil {
		h.fail(w, r, err)
		return
	}
	var permission *service.PermissionResult
	if body.Permission != nil {
		result, err := h.svc.SetPermission(r.Context(), principal, body.IDs, *body.Permission)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		permission = result
	}
	if body.AlbumID != nil || body.ClearAlbum {
		var albumID *string
		if !body.ClearAlbum {
			albumID = body.AlbumID
		}
		if err := h.svc.SetAlbum(r.Context(), principal, body.IDs, albumID); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	// 返回审查结果：blocked 为因 AI 审查未通过而保持私有的图片（仅公开操作时）。
	response := map[string]any{"updated": len(body.IDs)}
	if permission != nil {
		response["published"] = permission.Published
		if len(permission.Blocked) > 0 {
			response["blocked"] = permission.Blocked
		}
	}
	writeOK(w, response)
}

// batchDeleteRequest 是 POST /api/v1/images/batch-delete 的请求体。
type batchDeleteRequest struct {
	IDs []string `json:"ids"`
}

// batchDeleteImages 批量删除一组图片，返回成功与失败的数量。
func (h *Handler) batchDeleteImages(w http.ResponseWriter, r *http.Request) {
	var body batchDeleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	deleted, failed, err := h.svc.DeleteBatch(r.Context(), principalOf(r), body.IDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]any{"deleted": deleted, "failed": failed})
}

// serveImage 按以斜杠分隔的键流式传输已存储的对象，当查询参数要求时
// 应用即时的转换。
func (h *Handler) serveImage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	req, err := parseTransformQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
		return
	}
	// 要求存在已注册的元数据，这样即时转换就无法直接从存储后端
	// 提供任意对象。
	dto, backend, err := h.svc.GetByKeyWithBackend(r.Context(), key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if backend == nil {
		h.fail(w, r, service.ErrStorageConfig)
		return
	}
	if h.imaging == nil || !h.imaging.Enabled() || req.Empty() {
		h.serveOriginal(w, r, dto, backend)
		return
	}

	opts, err := h.imaging.Options(req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	etag := transformETag(key, opts)
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	result, err := h.imaging.Render(r.Context(), backend, key, opts)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(result.Data); err != nil {
		h.logger.WarnContext(r.Context(), "write processed image", slog.Any("error", err))
	}
}

// serveOriginal 原样流式传输已存储的对象。
func (h *Handler) serveOriginal(w http.ResponseWriter, r *http.Request, dto *service.ImageDTO, backend storage.Storage) {
	object, err := backend.Get(r.Context(), dto.Key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.fail(w, r, service.ErrNotFound)
			return
		}
		h.fail(w, r, err)
		return
	}
	defer func() {
		if closeErr := object.Close(); closeErr != nil {
			h.logger.WarnContext(r.Context(), "close stored object", slog.Any("error", closeErr))
		}
	}()

	w.Header().Set("Content-Type", dto.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dto.Size, 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, object); err != nil {
		// 状态行和头部已经发送，因此此处只能记录日志。
		h.logger.WarnContext(r.Context(), "stream stored object", slog.Any("error", err))
	}
}

// parseTransformQuery 从查询字符串中读取转换参数。
func parseTransformQuery(query url.Values) (service.TransformRequest, error) {
	var req service.TransformRequest
	var err error
	if req.Width, err = queryIntStrict(query, "w"); err != nil {
		return req, err
	}
	if req.Height, err = queryIntStrict(query, "h"); err != nil {
		return req, err
	}
	if req.Quality, err = queryIntStrict(query, "q"); err != nil {
		return req, err
	}
	if req.Rotate, err = queryIntStrict(query, "r"); err != nil {
		return req, err
	}
	if req.WatermarkOpacity, err = queryIntStrict(query, "wm_opacity"); err != nil {
		return req, err
	}
	if req.WatermarkSize, err = queryIntStrict(query, "wm_size"); err != nil {
		return req, err
	}
	if req.Blur, err = queryFloatStrict(query, "blur"); err != nil {
		return req, err
	}
	if req.Sharpen, err = queryFloatStrict(query, "sharpen"); err != nil {
		return req, err
	}
	req.Fit = query.Get("fit")
	req.Format = query.Get("f")
	req.Enlarge = parseBool(query.Get("enlarge"))
	req.Flip = query.Get("flip")
	req.Grayscale = parseBool(query.Get("gray"))
	if !req.Grayscale {
		req.Grayscale = parseBool(query.Get("grayscale"))
	}
	req.WatermarkText = query.Get("wm")
	req.WatermarkPosition = query.Get("wm_pos")
	req.WatermarkColor = query.Get("wm_color")
	return req, nil
}

func queryIntStrict(query url.Values, name string) (int, error) {
	raw := query.Get(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %q query parameter", name)
	}
	return value, nil
}

func queryFloatStrict(query url.Values, name string) (float64, error) {
	raw := query.Get(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %q query parameter", name)
	}
	return value, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// transformETag 为原始键加上转换返回一个稳定的 ETag。
func transformETag(key string, opts imaging.Options) string {
	sum := sha256.Sum256([]byte(key + "|" + imaging.OptionsDigest(opts)))
	return fmt.Sprintf(`"%x"`, sum[:16])
}

// queryInt 解析整数查询参数，若缺失或无效则返回 0，以便服务端
// 应用其默认值。
func queryInt(r *http.Request, name string) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return value
}
