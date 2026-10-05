# 图片处理

图片通过 `/i/<存储键>` 访问，可在查询串中附加处理参数：

```
http://localhost:8080/i/2026/10/05/6f1c…a2.png?w=400&h=300&fit=cover&f=webp&q=80
```

| 参数 | 含义 | 取值 |
|------|------|------|
| `w` | 目标宽度 | 像素 |
| `h` | 目标高度 | 像素 |
| `fit` | 缩放策略 | `contain`（默认，等比缩放）/ `cover`（裁剪填充）/ `fill`（拉伸填充，`cover`/`fill` 均需同时给出 `w`、`h`） |
| `q` | 输出质量 | 1–100，缺省用 `processing.default_quality` |
| `f` | 输出格式 | `jpeg`/`png`/`gif`/`webp`/`avif`（受 `processing.allowed_formats` 与处理器能力限制） |
| `r` | 旋转角度 | `90`/`180`/`270` |
| `flip` | 翻转 | `h`（水平）/`v`（垂直）/`hv`（同时） |
| `gray` | 转灰度 | `1`/`true` |
| `blur` | 高斯模糊半径 | 0–100 |
| `sharpen` | 锐化强度 | 0–100 |
| `wm` | 文字水印内容 | 任意文本（最长 200 字符） |
| `wm_pos` | 水印位置 | `top-left`/`top-right`/`bottom-left`/`bottom-right`/`center` |
| `wm_opacity` | 水印不透明度 | 0–100（默认 80） |
| `wm_size` | 水印字号 | 像素，0 表示自适应 |
| `wm_color` | 水印颜色 | 十六进制，如 `#ffffff` |
| `enlarge` | 是否允许放大 | `1`/`true`/`yes`/`on` |

带处理参数的响应会附带 `ETag`，支持 `If-None-Match` 返回 `304`。

`processing` 配置提供开关与默认值：

```yaml
processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
  allow_enlarge: false      # 是否允许放大
  allow_effects: true       # 是否允许灰度/模糊/锐化
  allow_watermark: true     # 是否允许通过 URL 加水印
  watermark_text: ""        # 非空时为所有变换强制叠加该水印
```

前端「图片处理」页面（`/processing`）提供上述参数的实时预览，并生成可复制的处理链接。

## 使用 libvips 处理器（可选）

默认使用纯 Go 处理器（支持 JPEG/PNG/GIF 输出）。如需 WebP/AVIF 输出与更高性能，可在安装 libvips 后使用：

```bash
go build -tags libvips -o bin/axmipic ./cmd/axmipic
```

## 图片处理驱动

`processing.driver` 选择运行时使用的处理器，未知或不可用的驱动会在启动时回退到 `purego`：

| 驱动 | 说明 | 依赖 |
|------|------|------|
| `purego` | 纯 Go（默认），支持 JPEG/PNG/GIF 输出 | 无 |
| `libvips` | libvips/bimg，额外支持 WebP/AVIF 与更高性能 | `-tags libvips` + CGO + libvips |
| `magick` | ImageMagick 命令行，额外支持 WebP/AVIF | 系统安装 `magick` 或 `convert` |

`GET /api/v1/admin/imaging/drivers` 返回当前二进制可用的驱动与正在使用的处理器。
