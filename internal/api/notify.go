package api

import (
	"encoding/json"
	"net/http"

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
