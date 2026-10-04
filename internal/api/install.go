package api

import (
	"encoding/json"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// installStatusResponse 描述当前安装状态。
type installStatusResponse struct {
	Installed bool     `json:"installed"`
	LockFile  string   `json:"lock_file,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

// installRequest 是安装向导提交的配置。
type installRequest struct {
	SiteName          string `json:"site_name"`
	BaseURL           string `json:"base_url"`
	DatabaseDriver    string `json:"database_driver"`
	DatabaseDSN       string `json:"database_dsn"`
	AdminUsername     string `json:"admin_username"`
	AdminPassword     string `json:"admin_password"`
	StorageDriver     string `json:"storage_driver"`
	StorageRoot       string `json:"storage_root"`
	AllowRegistration bool   `json:"allow_registration"`
	AllowGuestUpload  bool   `json:"allow_guest_upload"`
}

// installStatus 返回安装状态，供前端判断是否跳转到安装向导。
func (h *Handler) installStatus(w http.ResponseWriter, r *http.Request) {
	if h.install == nil {
		writeOK(w, installStatusResponse{Installed: true})
		return
	}
	status := h.install.Status()
	writeOK(w, installStatusResponse{Installed: status.Installed, LockFile: status.LockFile, Reasons: status.Reasons})
}

// runInstall 执行初始化。
func (h *Handler) runInstall(w http.ResponseWriter, r *http.Request) {
	if h.install == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "installer is not available")
		return
	}
	var body installRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	err := h.install.Install(r.Context(), service.InstallInput{
		SiteName:          body.SiteName,
		BaseURL:           body.BaseURL,
		DatabaseDriver:    body.DatabaseDriver,
		DatabaseDSN:       body.DatabaseDSN,
		AdminUsername:     body.AdminUsername,
		AdminPassword:     body.AdminPassword,
		StorageDriver:     body.StorageDriver,
		StorageRoot:       body.StorageRoot,
		AllowRegistration: body.AllowRegistration,
		AllowGuestUpload:  body.AllowGuestUpload,
	}, h.installRepo, h.installSeed)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]bool{"installed": true})
}
