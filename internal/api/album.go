package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

// listAlbums 返回当前主体可见的相册。
func (h *Handler) listAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := h.albums.List(r.Context(), principalOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, albums)
}

// albumRequest 是创建/更新相册的 JSON 请求体。
type albumRequest struct {
	Name  string `json:"name"`
	Intro string `json:"intro"`
}

// createAlbum 新建一个相册。
func (h *Handler) createAlbum(w http.ResponseWriter, r *http.Request) {
	var body albumRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	album, err := h.albums.Create(r.Context(), principalOf(r), service.AlbumInput{Name: body.Name, Intro: body.Intro})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, album)
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

// updateAlbum 修改相册的名称与简介。
func (h *Handler) updateAlbum(w http.ResponseWriter, r *http.Request) {
	var body albumRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	album, err := h.albums.Update(r.Context(), principalOf(r), chi.URLParam(r, "id"), service.AlbumInput{Name: body.Name, Intro: body.Intro})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, album)
}

// deleteAlbum 删除相册，其中的图片会被移出相册但保留。
func (h *Handler) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.albums.Delete(r.Context(), principalOf(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}
