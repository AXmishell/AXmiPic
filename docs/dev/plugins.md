# 插件系统

AXmiPic 支持运行时加载的**插件**：插件是被隔离的独立模块，宿主只向其暴露受控
能力（或按信任级别放行），因此可以在不重新编译、不重启主程序的情况下新增或更新
渠道。统一使用 `category + descriptor` 契约，通知、支付、存储等领域均可复用。

提供两种运行时：

| 运行时 | 隔离方式 | 适用场景 |
|--------|----------|----------|
| `wasm` | wazero 沙箱，仅可访问白名单主机 | 轻量、便于分发的渠道（短信宝 / 阿里云 / 腾讯云） |
| `process` | 独立子进程，stdio JSON 通信 | 需要官方云 SDK、长连接或任意 Go 库的重型渠道 |

WASM 插件用 Go + `sdk/plugin-go` 编写；进程插件用 Go + `sdk/plugin-go/process` 编写，
也可用任意语言实现同一 stdio 协议。

## 目录结构

```
plugins/                   插件根目录（默认，可配置 plugins.dir）
  smsbao/
    plugin.yaml            清单
    plugin.wasm            编译产物
internal/plugin/           宿主框架（加载、注册、沙箱、宿主能力）
sdk/plugin-go/             Go 插件 SDK
  process/                 进程插件 SDK（stdio JSON）
  examples/smsbao/         示例插件（短信宝，wasm）
  examples/aliyun-sms/     示例插件（阿里云短信，wasm）
  examples/tencent-sms/    示例插件（腾讯云短信，wasm）
  examples/process-webhook/ 示例插件（Webhook 转发，process）
```

每个 `plugins/<name>/` 子目录为一个插件，通过 `plugin.yaml` 描述。

## 清单 `plugin.yaml`

```yaml
name: smsbao              # 插件名（唯一）
version: 0.1.0
category: notify.sms      # 领域命名空间：notify.sms / notify.email / payment / ...
runtime: wasm             # 当前仅支持 wasm
entry: plugin.wasm        # 相对入口文件
abi: 1                    # 与宿主版本一致
capabilities:             # 能力声明，未声明即不可用
  http:
    hosts: ["api.smsbao.com", "*.example.com"]
# checksum: sha256:<hex>  # 可选，校验入口文件完整性
```

## 编写插件（Go SDK）

插件实现 `Provider` 接口并在 `init` 中注册：

```go
package main

import plugin "github.com/AXmishell/axmipic/sdk/plugin-go"

type myPlugin struct{ user string }

func (p *myPlugin) Describe() plugin.Descriptor {
    return plugin.Descriptor{Title: "示例", Fields: []plugin.Field{
        {Key: "user", Label: "账号", Required: true},
        {Key: "secret", Label: "密钥", Type: plugin.FieldPassword, Secret: true},
    }}
}

func (p *myPlugin) Configure(cfg map[string]string) error { p.user = cfg["user"]; return nil }

func (p *myPlugin) Invoke(op string, in []byte) ([]byte, error) {
    // 通过宿主发起受白名单约束的 HTTP 请求
    resp, err := plugin.HTTPDo(plugin.HTTPRequest{Method: "GET", URL: "https://api.example.com/send"})
    if err != nil { return nil, err }
    _ = resp
    return []byte(`{"ok":true}`), nil
}

func init() { plugin.Register(&myPlugin{}) }

func main() {}
```

构建为 WASM reactor（必须 `package main` 且包含空的 `func main(){}`）：

```bash
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 \
  go build -buildmode=c-shared -o plugin.wasm .
```

> `-buildmode=c-shared` 会导出 `_initialize`，宿主据此初始化 Go 运行时后再调用
> ABI 函数。使用普通 `go build` 会导出 `_start` 并直接退出，无法作为插件加载。

## ABI

插件必须导出：

| 导出 | 说明 |
|------|------|
| `alloc(size) -> ptr` | 分配 guest 内存 |
| `dealloc(ptr, size)` | 释放 guest 内存 |
| `describe() -> i64` | 返回自描述 JSON 的 `(ptr<<32\|len)` |
| `configure(ptr, len) -> i64` | 注入配置，返回结果信封 |
| `invoke(opPtr, opLen, inPtr, inLen) -> i64` | 执行操作，返回结果信封 |

宿主向插件提供导入模块 `axmipic`：

| 导入 | 说明 |
|------|------|
| `http_request(ptr, len) -> i64` | 唯一网络出口，强制主机白名单、方法、体积与超时 |
| `log(level, ptr, len)` | 结构化日志 |
| `now_millis() -> i64` | 当前 Unix 毫秒时间戳 |

结果统一使用信封 `{"result":<base64>, "error":"..."}` 编码，`ptr/len` 以
`(ptr<<32|len)` 打包为 `i64`。这些细节由 SDK 封装，插件作者无需手动处理。

## 进程外插件（process）

进程插件是独立可执行文件，通过 stdin/stdout 上的换行分隔 JSON 请求/响应与宿主
通信，可使用任意 Go 库（含云厂商官方 SDK）与网络。

清单：

```yaml
name: process-webhook
category: notify.sms
runtime: process
entry: plugin-bin     # 插件目录内的可执行文件名
abi: 1
```

用 `sdk/plugin-go/process` 编写：

```go
package main

import (
    plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
    "github.com/AXmishell/axmipic/sdk/plugin-go/process"
)

type impl struct{ url string }

func (p *impl) Describe() plugin.Descriptor { /* ... */ }
func (p *impl) Configure(cfg map[string]string) error { p.url = cfg["url"]; return nil }
func (p *impl) Invoke(op string, in []byte) ([]byte, error) { /* 可用 net/http、云 SDK 等 */ }

func main() { process.Main(&impl{}) }
```

构建（宿主架构，非 wasm）：`go build -o plugin-bin .`

协议：请求为 `{"id":N,"method":"describe|configure|invoke|close","params":...}`，
响应为 `{"id":N,"result":...,"error":"..."}`；`invoke` 的输入/输出字节以 base64
编码。插件日志请写入 stderr。

> ⚠️ 进程插件**运行在宿主同一权限下，不受 WASM 沙箱与 HTTP 白名单约束**，请仅
> 从可信来源安装。宿主以最小环境变量启动子进程，并在加载时记录告警。

## 配置与加载

```yaml
plugins:
  enabled: true
  dir: "./plugins"
  http_timeout_sec: 10
  max_http_body_kb: 1024
  trusted_keys: []          # Ed25519 可信公钥（base64）；设置后安装必须验签
  require_signature: false  # 为 true 时无有效签名一律拒绝
  index_url: ""             # 插件市场索引（JSON）地址
  max_archive_mb: 64        # 归档解压后的最大体积
```

启动时扫描 `plugins.dir`，逐个校验清单与校验和、加载模块并注册。单个插件失败
不会阻止其它插件或服务启动。

## 后台管理接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/plugins?category=` | 列出已启用插件自描述（按类别可选） |
| GET | `/api/v1/admin/plugins/installed` | 列出全部已安装插件及启用/配置状态 |
| GET | `/api/v1/admin/plugins/registry` | 拉取插件市场索引 |
| POST | `/api/v1/admin/plugins/install` | 安装插件（JSON 按 URL/索引名，或原始 body 为归档） |
| GET | `/api/v1/admin/plugins/{name}` | 读取插件配置；秘钥字段仅返回是否已设置 |
| PUT | `/api/v1/admin/plugins/{name}/config` | 保存配置并即时应用（秘钥以密文存储） |
| PUT | `/api/v1/admin/plugins/{name}/enabled` | 启用/暂停插件（`{"enabled":bool}`，状态持久化） |
| POST | `/api/v1/admin/plugins/{name}/test` | 用当前配置发送一条测试通知 |
| POST | `/api/v1/admin/plugins/{name}/reload` | 重新加载插件（保持启用状态） |
| DELETE | `/api/v1/admin/plugins/{name}` | 卸载并删除插件 |

后台「短信渠道」按插件的 `fields` 渲染动态表单：普通字段直接展示，`secret`
字段以密码框呈现且不回显；保存后可用「测试发送」在切换正式渠道前验证。

## 接入通知渠道

短信设置域的 `channel` 决定渠道来源：

- `log`：仅记录日志，便于本地调试；
- `http`：使用通用 HTTP 网关（`provider`/`endpoint`/`method`）；
- 其它值：视为已加载的短信插件名（如 `smsbao`）。

选择插件作为渠道时，宿主会用已保存的配置 `Configure` 插件实例，并把
`notify.Sender` 桥接到插件的 `invoke("send", ...)`；渠道切换在保存设置后即时
生效，无需重启。插件收到的载荷为中性消息（`to`/`subject`/`body`/`template`/
`params`/`sign_name`），由插件映射到具体服务商。

## 在线安装与签名

插件归档为 zip（根目录含 `plugin.yaml` 与入口文件）。发布工具 `pluginpack`
位于 `sdk/plugin-go/cmd/pluginpack`：

```bash
cd sdk/plugin-go
# 打包并签名（私钥为 base64 的 32 字节种子或 64 字节 Ed25519 私钥）
go run ./cmd/pluginpack -dir ../../plugins/smsbao -out smsbao-0.1.0.zip -key "$PRIVATE_KEY"
# 输出 sha256=... 与 signature=...
```

安装方式：

- **本地上传**：`POST /api/v1/admin/plugins/install`，body 为 zip 原始字节，可选头部
  `X-Plugin-Sha256`、`X-Plugin-Signature`。
- **按 URL / 索引名**：JSON body `{"url":"...","sha256":"...","signature":"..."}`，
  或 `{"name":"smsbao"}` 配合 `index_url` 从市场解析。

启用/暂停：`PUT /api/v1/admin/plugins/{name}/enabled`，body `{"enabled":true|false}`。
暂停会注销并（磁盘插件）关闭其实例，但保留文件与配置；状态持久化，重启后仍生效。
暂停的插件不出现在短信渠道选择列表中，但其描述与配置仍可查看/编辑；重新启用时会
自动重新加载并尽力应用已保存配置。

宿主会校验大小、sha256 与 Ed25519 签名（配置了 `trusted_keys` 或
`require_signature` 时强制），随后安全解压（拒绝路径穿越、符号链接与超限内容），
校验清单并热加载；进程插件入口自动赋予可执行位。

市场索引 `index_url` 的 JSON 结构：

```json
{
  "plugins": [
    {
      "name": "smsbao",
      "version": "0.1.0",
      "category": "notify.sms",
      "runtime": "wasm",
      "description": "短信宝",
      "url": "https://example.com/smsbao-0.1.0.zip",
      "sha256": "…",
      "signature": "…"
    }
  ]
}
```

## 安全模型

- WASM 沙箱：限制线性内存（默认 64 MiB），支持按上下文超时中断。
- 能力白名单：`capabilities.http.hosts` 之外的主机一律拒绝；未声明则完全禁网。
- 进程插件不受沙箱约束，运行在宿主权限下，需自行保证来源可信（见上文警告）。
- 秘钥字段由宿主以 AES-256-GCM 加密后写入 `settings` 表，AAD 绑定插件名与字段名，
  接口返回值绝不包含明文。
- `checksum` 可选校验，防止篡改入口文件。

## 构建与测试

```bash
make plugin-example   # 构建短信宝示例插件到 plugins/smsbao
make test-plugin      # 运行插件框架测试（含 WASM 端到端）
```

## 路线图

1. ~~**M2**：把插件接入 `notify`、管理端动态表单与凭据加密、测试发送。~~ ✅
2. ~~**M3**：阿里云 / 腾讯云短信插件。~~ ✅（见 `sdk/plugin-go/examples/`）
3. ~~**M4**：进程外运行时，承载需要云 SDK 的重型渠道。~~ ✅（stdio JSON）
4. ~~**M5**：在线安装、Ed25519 签名校验与插件市场索引。~~ ✅

## 内置示例插件

| 插件 | 目录 | 运行时 | 说明 |
|------|------|--------|------|
| 短信宝 | `examples/smsbao` | wasm | 通用 HTTP + MD5 签名 |
| 阿里云短信 | `examples/aliyun-sms` | wasm | RPC + HMAC-SHA1 签名，模板参数取 `params` |
| 腾讯云短信 | `examples/tencent-sms` | wasm | TC3-HMAC-SHA256 签名，JSON POST |
| Webhook | `examples/process-webhook` | process | JSON POST 转发，演示 net/http |

云厂商插件均使用 Go 标准库在插件内完成签名，宿主无需引入任何厂商依赖。运行
`make plugin-example` 会把全部示例构建到 `plugins/`，随后可在后台「短信渠道」选择。
