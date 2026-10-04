package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

type announcementRequest struct {
	Title     string `json:"title"`
	Content   string `json:"content"`
	Level     string `json:"level"`
	Pinned    bool   `json:"pinned"`
	Published bool   `json:"published"`
}

type reportRequest struct {
	ImageID string `json:"image_id"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail"`
}

type reportUpdateRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

type pageRequest struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Published bool   `json:"published"`
}

// ---- 公开 ----

// listAnnouncements 返回已发布的公告。
func (h *Handler) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	items, err := h.site.ListAnnouncements(r.Context(), true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, items)
}

// getPage 按 slug 返回已发布的独立页面。
func (h *Handler) getPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.site.GetPageBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, page)
}

// createReport 提交举报。
func (h *Handler) createReport(w http.ResponseWriter, r *http.Request) {
	var body reportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	report, err := h.site.CreateReport(r.Context(), principalOf(r), service.ReportInput{
		ImageID: body.ImageID,
		Reason:  body.Reason,
		Detail:  body.Detail,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, report)
}

// ---- 管理：公告 ----

func (h *Handler) adminListAnnouncements(w http.ResponseWriter, r *http.Request) {
	items, err := h.site.ListAnnouncements(r.Context(), false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, items)
}

func (h *Handler) adminCreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeAnnouncement(w, r)
	if !ok {
		return
	}
	item, err := h.site.CreateAnnouncement(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, item)
}

func (h *Handler) adminUpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeAnnouncement(w, r)
	if !ok {
		return
	}
	item, err := h.site.UpdateAnnouncement(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, item)
}

func (h *Handler) adminDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.site.DeleteAnnouncement(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// ---- 管理：举报 ----

func (h *Handler) adminListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := h.site.ListReports(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, reports)
}

func (h *Handler) adminUpdateReport(w http.ResponseWriter, r *http.Request) {
	var body reportUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	report, err := h.site.UpdateReportStatus(r.Context(), chi.URLParam(r, "id"), body.Status, body.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, report)
}

// ---- 管理：页面 ----

func (h *Handler) adminListPages(w http.ResponseWriter, r *http.Request) {
	pages, err := h.site.ListPages(r.Context(), false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, pages)
}

func (h *Handler) adminCreatePage(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePage(w, r)
	if !ok {
		return
	}
	page, err := h.site.CreatePage(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, page)
}

func (h *Handler) adminUpdatePage(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePage(w, r)
	if !ok {
		return
	}
	page, err := h.site.UpdatePage(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, page)
}

func (h *Handler) adminDeletePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.site.DeletePage(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

func decodeAnnouncement(w http.ResponseWriter, r *http.Request) (service.AnnouncementInput, bool) {
	var body announcementRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.AnnouncementInput{}, false
	}
	return service.AnnouncementInput{
		Title:     body.Title,
		Content:   body.Content,
		Level:     body.Level,
		Pinned:    body.Pinned,
		Published: body.Published,
	}, true
}

func decodePage(w http.ResponseWriter, r *http.Request) (service.PageInput, bool) {
	var body pageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.PageInput{}, false
	}
	return service.PageInput{
		Slug:      body.Slug,
		Title:     body.Title,
		Content:   body.Content,
		Published: body.Published,
	}, true
}
