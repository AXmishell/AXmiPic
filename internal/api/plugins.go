package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/service"
)

// maxPluginArchive 是上传插件归档的最大字节数。
const maxPluginArchive = 64 << 20

// adminListPlugins 返回已加载插件的自描述，可按 category 过滤，供后台渲染
// 各插件的配置表单。
func (h *Handler) adminListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeOK(w, map[string]any{"items": []plugin.Descriptor{}})
		return
	}
	items := h.plugins.Descriptors(r.URL.Query().Get("category"))
	if items == nil {
		items = []plugin.Descriptor{}
	}
	writeOK(w, map[string]any{"items": items})
}

// adminGetPluginConfig 返回指定插件的配置（秘钥字段仅返回是否已设置）。
func (h *Handler) adminGetPluginConfig(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	cfg, err := h.plugins.Config(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

type pluginConfigRequest struct {
	Values map[string]string `json:"values"`
}

// adminUpdatePluginConfig 保存插件配置并即时应用（无需重启）。
func (h *Handler) adminUpdatePluginConfig(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	var body pluginConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.plugins.SaveConfig(r.Context(), name, body.Values); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.plugins.Apply(r.Context(), name); err != nil {
		h.fail(w, r, err)
		return
	}
	cfg, err := h.plugins.Config(r.Context(), name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

type pluginTestRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// adminTestPlugin 用当前已保存的配置向插件发送一条测试通知。
func (h *Handler) adminTestPlugin(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	var body pluginTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	result, err := h.plugins.Test(r.Context(), chi.URLParam(r, "name"), body.To, body.Subject, body.Body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]any{"status": "sent", "result": json.RawMessage(nonEmptyJSON(result))})
}

// nonEmptyJSON 在结果为空时返回 JSON null，避免拼接非法 JSON。
func nonEmptyJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	return raw
}

// adminPluginRegistry 返回插件市场索引。
func (h *Handler) adminPluginRegistry(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	items, err := h.plugins.Registry(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if items == nil {
		items = []plugin.MarketEntry{}
	}
	writeOK(w, map[string]any{"items": items, "configured": h.plugins.RegistryConfigured()})
}

// adminInstallPlugin 安装插件：JSON 请求体按 URL/索引安装，原始 body 视为上传归档。
func (h *Handler) adminInstallPlugin(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var spec plugin.InstallSpec
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&spec); err != nil {
			writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
			return
		}
		man, err := h.plugins.Install(r.Context(), spec)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeOK(w, man)
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPluginArchive))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, "archive too large")
		return
	}
	man, err := h.plugins.InstallArchive(r.Context(), data,
		r.Header.Get("X-Plugin-Sha256"), r.Header.Get("X-Plugin-Signature"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, man)
}

// adminRemovePlugin 卸载并删除指定插件。
func (h *Handler) adminRemovePlugin(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.plugins.Remove(r.Context(), name); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "removed", "name": name})
}

// adminReloadPlugin 重新加载指定插件并返回其配置。
func (h *Handler) adminReloadPlugin(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.plugins.Reload(r.Context(), name); err != nil {
		h.fail(w, r, err)
		return
	}
	cfg, err := h.plugins.Config(r.Context(), name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

// adminInstalledPlugins 返回全部已安装插件的启用与配置状态。
func (h *Handler) adminInstalledPlugins(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeOK(w, map[string]any{"items": []service.PluginStatus{}})
		return
	}
	items := h.plugins.Installed(r.Context())
	if items == nil {
		items = []service.PluginStatus{}
	}
	writeOK(w, map[string]any{"items": items})
}

type pluginEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// adminSetPluginEnabled 启用或暂停指定插件，并持久化状态。
func (h *Handler) adminSetPluginEnabled(w http.ResponseWriter, r *http.Request) {
	if h.plugins == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plugins unavailable")
		return
	}
	var body pluginEnabledRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := chi.URLParam(r, "name")
	if err := h.plugins.SetEnabled(r.Context(), name, body.Enabled); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]any{"name": name, "enabled": body.Enabled})
}
