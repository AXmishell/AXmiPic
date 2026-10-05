package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

// listAlbums 返回当前主体可见的相册。
func (h *Handler) listAlbums(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureAlbums); err != nil {
		h.fail(w, r, err)
		return
	}
	albums, err := h.albums.List(r.Context(), principal)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, albums)
}

// albumRequest 是创建/更新相册的 JSON 请求体。
type albumRequest struct {
	Name       string `json:"name"`
	Intro      string `json:"intro"`
	Permission string `json:"permission"`
}

// createAlbum 新建一个相册。
func (h *Handler) createAlbum(w http.ResponseWriter, r *http.Request) {
	var body albumRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureAlbums); err != nil {
		h.fail(w, r, err)
		return
	}
	album, err := h.albums.Create(r.Context(), principal, service.AlbumInput{
		Name: body.Name, Intro: body.Intro, Permission: body.Permission,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, album)
}

// listPublicAlbums 返回公开相册，可按所有者过滤。
func (h *Handler) listPublicAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.albums.ListPublic(r.Context(), r.URL.Query().Get("user_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, albums)
}

// getAlbum 返回单个相册。
func (h *Handler) getAlbum(w http.ResponseWriter, r *http.Request) {
	album, err := h.albums.Get(r.Context(), principalOf(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, album)
}

// listAlbumImages 返回相册中的图片，公开相册无需登录即可浏览。
func (h *Handler) listAlbumImages(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.ListAlbumImages(
		r.Context(),
		principalOf(r),
		chi.URLParam(r, "id"),
		queryInt(r, "page"),
		queryInt(r, "page_size"),
	)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

// publicProfile 返回某个用户的公开资料与公开相册。
func (h *Handler) publicProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.albums.PublicProfile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, profile)
}

// updateAlbum 修改相册的名称、简介与可见性。
func (h *Handler) updateAlbum(w http.ResponseWriter, r *http.Request) {
	var body albumRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureAlbums); err != nil {
		h.fail(w, r, err)
		return
	}
	album, err := h.albums.Update(r.Context(), principal, chi.URLParam(r, "id"), service.AlbumInput{
		Name: body.Name, Intro: body.Intro, Permission: body.Permission,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, album)
}

// deleteAlbum 删除相册，其中的图片会被移出相册但保留。
func (h *Handler) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	principal := principalOf(r)
	if err := h.requireFeature(r, principal, service.FeatureAlbums); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.albums.Delete(r.Context(), principal, id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}
