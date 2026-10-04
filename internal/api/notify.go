package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/AXmishell/axmipic/internal/imaging"
)

type notifyTestRequest struct {
	Channel string `json:"channel"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// adminNotifyChannels 返回已配置的通知渠道。
func (h *Handler) adminNotifyChannels(w http.ResponseWriter, r *http.Request) {
	sms, email := h.notify.Channels()
	writeOK(w, map[string]string{"sms": sms, "email": email})
}

// adminTestNotify 发送一条测试通知，用于验证短信/邮件配置。
func (h *Handler) adminTestNotify(w http.ResponseWriter, r *http.Request) {
	var body notifyTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var err error
	switch body.Channel {
	case "sms":
		err = h.notify.SendSMS(r.Context(), body.To, body.Body)
	case "email":
		err = h.notify.SendEmail(r.Context(), body.To, body.Subject, body.Body)
	default:
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "channel must be sms or email")
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "sent"})
}

// adminSecurityInfo 暴露当前安全扫描器名称，供管理端展示。
func (h *Handler) adminSecurityInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{}
	if h.svc != nil {
		info["scanner"] = h.svc.ScannerName()
	}
	writeOK(w, info)
}

// adminImagingDrivers 返回当前二进制可用的图片处理驱动。
func (h *Handler) adminImagingDrivers(w http.ResponseWriter, r *http.Request) {
	drivers := imaging.AvailableDrivers()
	names := make([]string, 0, len(drivers))
	for _, d := range drivers {
		names = append(names, string(d))
	}
	active := ""
	if h.imaging != nil {
		active = h.imaging.Capabilities().Name
	}
	writeOK(w, map[string]any{"available": names, "active": active})
}

// RuntimeInfo 描述实例的运行时与运行环境信息（不含任何密钥）。
type RuntimeInfo struct {
	SiteName          string   `json:"site_name"`
	BaseURL           string   `json:"base_url"`
	DatabaseDriver    string   `json:"database_driver"`
	StorageDriver     string   `json:"storage_driver"`
	Processor         string   `json:"processor"`
	Formats           []string `json:"formats"`
	AllowRegistration bool     `json:"allow_registration"`
	RequireAuth       bool     `json:"require_auth"`
	AllowGuestUpload  bool     `json:"allow_guest_upload"`
	GuestQuotaMB      int      `json:"guest_quota_mb"`
	GuestUploadMaxMB  int      `json:"guest_upload_max_mb"`
	DefaultQuotaMB    int      `json:"default_quota_mb"`
	UploadMaxMB       int      `json:"upload_max_mb"`
	TrustProxy        bool     `json:"trust_proxy"`
	SessionTTLHours   int      `json:"session_ttl_hours"`
	InstallLockFile   string   `json:"install_lock_file"`
	Installed         bool     `json:"installed"`
	GoVersion         string   `json:"go_version"`
	Platform          string   `json:"platform"`
}

// adminRuntimeInfo 返回实例运行环境信息。
func (h *Handler) adminRuntimeInfo(w http.ResponseWriter, r *http.Request) {
	info := h.runtime
	if h.install != nil {
		status := h.install.Status()
		info.Installed = status.Installed
		if info.InstallLockFile == "" {
			info.InstallLockFile = status.LockFile
		}
	}
	writeOK(w, info)
}

// ProcessInfo 描述进程的实时运行时指标。
type ProcessInfo struct {
	// Goroutines 为当前 goroutine 数量。
	Goroutines int `json:"goroutines"`
	// HeapAlloc 为已分配且仍在使用的堆内存字节数。
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	// HeapInuse 为正在使用的堆内存字节数（含尚未释放的 span）。
	HeapInuseBytes uint64 `json:"heap_inuse_bytes"`
	// HeapObjects 为存活堆对象数量。
	HeapObjects uint64 `json:"heap_objects"`
	// HeapSys 为从操作系统保留的堆内存字节数。
	HeapSysBytes uint64 `json:"heap_sys_bytes"`
	// Sys 为从操作系统获取的虚拟内存总字节数。
	SysBytes uint64 `json:"sys_bytes"`
	// StackInuse 为栈内存使用字节数。
	StackInuseBytes uint64 `json:"stack_inuse_bytes"`
	// GCCount 为已完成的 GC 次数。
	GCCount uint32 `json:"gc_count"`
	// NumCPU 为可用逻辑 CPU 数量。
	NumCPU int `json:"num_cpu"`
	// UptimeSeconds 为进程已运行秒数。
	UptimeSeconds int64 `json:"uptime_seconds"`
	// GoVersion 与 Platform 为运行环境信息。
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// processStartedAt 记录进程启动时间，用于计算运行时长。
var processStartedAt = time.Now()

// adminProcessInfo 返回进程的实时运行时指标（Goroutine、堆内存、堆对象数等）。
func (h *Handler) adminProcessInfo(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeOK(w, ProcessInfo{
		Goroutines:      runtime.NumGoroutine(),
		HeapAllocBytes:  mem.HeapAlloc,
		HeapInuseBytes:  mem.HeapInuse,
		HeapObjects:     mem.HeapObjects,
		HeapSysBytes:    mem.HeapSys,
		SysBytes:        mem.Sys,
		StackInuseBytes: mem.StackInuse,
		GCCount:         mem.NumGC,
		NumCPU:          runtime.NumCPU(),
		UptimeSeconds:   int64(time.Since(processStartedAt).Seconds()),
		GoVersion:       runtime.Version(),
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
	})
}
