# 图片广场 AI 审查

对用户上传的图片，可在其「开放到图片广场」（设为公开）之前调用视觉模型做内容审查。审查**仅作用于图片广场**：未通过审查的图片不会公开，仍保存在用户自己的私有图片中。

配置方式有两种，**数据库中的设置优先**：

1. **后台在线配置（推荐，保存即时生效、无需重启）**：「系统设置 → AI 审查」页面，或 `GET/PUT /api/v1/admin/moderation`。密钥经主密钥加密后保存在数据库 `settings` 表，接口仅返回是否已设置（`api_key_set`）。
2. **配置文件 / 环境变量（首次启动的兜底）**：`moderation.*` 段或 `AXMIPIC_MODERATION_*`。数据库无记录时以它初始化并写入数据库。

| 配置 | 说明 |
|------|------|
| `moderation.enabled` | 是否启用图片广场 AI 审查 |
| `moderation.base_url` | 标准 OpenAI 兼容接口根地址，如 `https://api.openai.com/v1` |
| `moderation.api_key` | 接口密钥（Bearer）；在线保存时留空表示保持原密钥 |
| `moderation.model` | 视觉模型名，如 `gpt-4o-mini` |
| `moderation.timeout_sec` | 单次审查请求超时（秒，默认 30） |
| `moderation.prompt` | 覆盖默认审查提示词（默认要求模型只回答 `SAFE` 或 `UNSAFE`） |
| `moderation.max_image_mb` | 送审图片体积上限（MiB，默认 10）；超过按未通过处理 |

行为与兜底：

- 图片通过 `POST /api/v1/images/batch` 设为公开时逐张审查（并发上限 4），通过的公开、未通过的强制保持私有；响应会附带 `published` 与 `blocked`（被拦下的图片 id），前端据此提示用户。
- **保守拒绝**：模型返回空内容、无法识别，或接口调用失败（网络/超时/非 200）时，一律视为审查不通过，图片保持私有。
- 已经是公开的图片不会重复审查；将图片设为私有不触发审查。
- 该审查是「公开前」的闸门，不改变上传本身——图片始终先私有入库。
- 后台保存后会即时重建运行中的审查器，无需重启。

示例：

```yaml
moderation:
  enabled: true
  base_url: "https://api.openai.com/v1"
  api_key: "sk-..."
  model: "gpt-4o-mini"
  timeout_sec: 30
  max_image_mb: 10
```
