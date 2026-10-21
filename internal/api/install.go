package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

// installStatusResponse 描述当前初始化状态。
type installStatusResponse struct {
	Installed bool     `json:"installed"`
	LockFile  string   `json:"lock_file,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

// installRequest 是初始化模式提交的配置。
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

// installStatus 返回初始化状态，供前端初始化页面判断是否已完成。
func (h *Handler) installStatus(w http.ResponseWriter, r *http.Request) {
	if h.install == nil {
		writeOK(w, installStatusResponse{Installed: true})
		return
	}
	status := h.install.Status()
	writeOK(w, installStatusResponse{Installed: status.Installed, LockFile: status.LockFile, Reasons: status.Reasons})
}

// runInstall 执行初始化。初始化接口仅依赖锁文件判断是否已完成初始化。
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
	// 初始化会把权限开关写入目标数据库；立即重新加载一次，使同一进程无需重启
	// 即可应用「允许注册 / 允许访客上传」等运行设置。安装服务已把新数据库连接
	// 接管给运行中的仓库，因此管理员登录也从目标库读取。
	if h.settings != nil {
		if err := h.settings.BootstrapAuth(r.Context()); err != nil {
			h.logger.WarnContext(r.Context(), "reload auth settings after initialization", slog.Any("error", err))
		}
	}
	if h.adminEntry != nil {
		if err := h.adminEntry.Bootstrap(r.Context()); err != nil {
			h.logger.WarnContext(r.Context(), "reload admin entry settings after initialization", slog.Any("error", err))
		}
	}
	writeOK(w, map[string]bool{"installed": true})
}
