package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

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

// runInstall 执行初始化。未安装时仅允许本机或私网来源调用，避免服务在初始化
// 前被公网访问时被他人抢先完成安装并接管实例。
func (h *Handler) runInstall(w http.ResponseWriter, r *http.Request) {
	if h.install == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "installer is not available")
		return
	}
	if !h.install.IsInstalled() && !installAllowedFrom(r) {
		writeError(w, http.StatusForbidden, http.StatusForbidden, "installation is only allowed from a local or private network")
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

// installAllowedFrom 报告安装请求是否来自本机或私网地址。无法解析来源地址时
// 拒绝，避免在来源不可信时放行安装。
func installAllowedFrom(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
