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
  memory_limit_mb: 64       # 每个 WASM 插件线性内存上限（MiB）；0=内置默认 64
  idle_unload_minutes: 0    # 空闲自动卸载时长（分钟）；0=关闭
  startup: "lazy"           # lazy（默认）仅登记不实例化；eager 启动即实例化
  preload: []               # 始终预热的插件名（如活跃的短信渠道）
```

启动时扫描 `plugins.dir`，逐个校验清单与校验和。默认采用 **lazy** 模式：只登记
已安装插件而**不实例化**，首次使用时才真正加载；单个插件失败不会阻止其它插件或
服务启动。

## 插件生命周期与内存管理

宿主把插件的「**逻辑启用**」与「**物理加载**」分开，由此定义三态（对外字段
`state`）：

| `state` | 逻辑启用 | 物理加载 | 含义 |
|---------|:---:|:---:|------|
| `disabled` | ❌ | ❌ | 已暂停（后台关闭）；不加载、不占内存 |
| `standby` | ✅ | ❌ | 已启用但**待激活**：未实例化，首次调用才加载 |
| `active` | ✅ | ✅ | 运行中：已实例化并占用内存 |

- 持久化只存**意图**：`settings` 的 `plugin.state.<name>` ∈ `enabled` / `disabled`。
- `active` 是纯运行时态，进程重启后回到 `standby`，不落库。
- 因此「启用」≠「占用内存」：**只有真正被使用（或被 preload）时才 active**。

完整生命周期：

1. **启动（lazy，默认）**：扫描目录时先只读 `plugin.yaml`（不实例化），所有插件
   登记为 `standby`/`disabled`，启动瞬间不创建任何 wazero runtime。若配置
   `startup: "eager"`，则启动即实例化所有 `enabled` 插件；无论哪种模式，`preload`
   列表中的插件都会立即激活。
2. **按需激活**：`Invoke`、`Configure`、构造通知渠道（`Sender`）、`Test` 或
   `PUT .../reload` 时，若插件为 `standby` 则就地实例化并转为 `active`。首次调用有
   一次冷启动开销（WASM 解码 + Go 运行时初始化）。
3. **启用/暂停**：`PUT .../enabled {"enabled":true}` 仅把插件置为 `standby`（**不加载**）；
   `{"enabled":false}` 置为 `disabled` 并若已加载则立即 `Close` 释放内存。文件与配置
   保留，状态持久化，重启后仍生效。
4. **空闲自动卸载**：`plugins.idle_unload_minutes > 0` 时，`active` 但超过该时长未被
   调用的磁盘插件会被卸载（回到 `standby`，**保持启用**）；再次调用透明重载。进程内
   注册的 Provider 不会被空闲卸载。
5. **描述缓存**：卸载/暂停后，插件的 `descriptor`（字段、标题）缓存于 `settings`
   的 `plugin.desc.<name>`，后台表单与渠道列表仍可正常展示，不会因未加载而丢字段。
6. **关闭**：进程退出时关闭全部已加载实例。

并发安全：`Close` 与在途的 `Invoke`/`Configure` 通过互斥锁串行化；同一插件的首次
加载也在锁内双检，保证「调用进行中暂停」或并发首次调用不会重复实例化或并发关闭
wazero runtime。激活失败时插件保持 `standby` 并记录 `last_error`，不会误改启用意图。

内存与状态可见性：

- `GET /api/v1/admin/plugins/installed` 返回每个插件的 `state`（disabled/standby/active）、
  `enabled`、`loaded`、`memory_bytes`（已加载 WASM 插件的线性内存字节数；`process`
  运行时为 0）、`last_used`（Unix 秒，0 表示加载后尚未调用）与 `last_error`。
- `GET /api/v1/admin/plugins?category=` 只返回**已启用**（`standby`/`active`）的插件；
  暂停的不出现在渠道选择列表中，但其描述与配置仍可通过 `/installed` 查看/编辑。
- 结合管理端「进程信息」接口与可选的 pprof 端点，可观察整体内存占用。

> 实践建议：`lazy` 默认下，未被调用的插件完全不占内存。若只使用一个短信渠道，把
> 其余渠道**暂停**可彻底不加载；把活跃渠道加入 `preload` 或设为 `idle_unload_minutes`
> （如 `30`~`60`）可在「秒开」与「省内存」之间按需取舍。

## 后台管理接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/plugins?category=` | 列出已启用插件自描述（按类别可选） |
| GET | `/api/v1/admin/plugins/installed` | 列出全部已安装插件及**状态**：`state`(disabled/standby/active)、`enabled`、`loaded`、`memory_bytes`、`last_used`、`last_error`、`configured` |
| GET | `/api/v1/admin/plugins/registry` | 拉取插件市场索引 |
| POST | `/api/v1/admin/plugins/install` | 安装插件（JSON 按 URL/索引名，或原始 body 为归档） |
| GET | `/api/v1/admin/plugins/{name}` | 读取插件配置；秘钥字段仅返回是否已设置 |
| PUT | `/api/v1/admin/plugins/{name}/config` | 保存配置并即时应用（秘钥以密文存储） |
| PUT | `/api/v1/admin/plugins/{name}/enabled` | 启用/暂停插件（`{"enabled":bool}`，状态持久化） |
| POST | `/api/v1/admin/plugins/{name}/test` | 用当前配置发送一条测试通知 |
| POST | `/api/v1/admin/plugins/{name}/reload` | 重新加载插件（保持启用状态） |
| DELETE | `/api/v1/admin/plugins/{name}` | 卸载并删除插件 |

后台「插件市场」是插件配置的唯一入口：按插件的 `fields` 渲染动态表单，普通字段直接展示，`secret` 字段以密码框呈现且不回显；保存后可用「测试发送」在切换正式渠道前验证。

> 「短信渠道」设置页**不再内联插件配置表单**，只负责选择渠道，并展示该插件的就绪状态（已暂停 / 待激活 / 运行中、已配置 / 未配置）与「去插件市场配置」入口。

## 接入通知渠道

短信设置域的 `channel` 决定渠道来源：

- `log`：仅记录日志，便于本地调试；
- `http`：使用通用 HTTP 网关（`provider`/`endpoint`/`method`）；
- 其它值：视为已加载的短信插件名（如 `smsbao`）。

选择插件作为渠道时，宿主会用已保存的配置 `Configure` 插件实例，并把
`notify.Sender` 桥接到插件的 `invoke("send", ...)`；渠道切换在保存设置后即时
生效，无需重启。插件收到的载荷为中性消息（`to`/`subject`/`body`/`template`/
`params`/`sign_name`），由插件映射到具体服务商。

**配置入口唯一化**：插件字段只在「插件市场」页配置，短信渠道页仅选择渠道。保存
短信设置时会校验所选插件**已安装且已启用**（未安装/已暂停会拒绝保存并返回错误），
避免静默回退；若插件尚未填写配置，页面会给出提示并可一键跳转到插件市场（
`/admin/plugins?config=<name>` 会自动打开该插件的配置弹窗）。保存后页面会刷新
实际生效渠道，若因插件未就绪而回退为日志渠道会明确告警。

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
在 `lazy` 模式下，`{"enabled":true}` 只把插件置为 `standby`（**不会立即加载**），
首次使用时才激活；`{"enabled":false}` 会注销并（磁盘插件）关闭其实例以**立即释放
内存**，但保留文件与配置。状态持久化，重启后仍生效。暂停的插件不出现在短信渠道
选择列表中，但其描述与配置仍可查看/编辑（来自 `plugin.desc.<name>` 缓存）。已启用
插件的空闲自动卸载见上文「插件生命周期与内存管理」。

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

- WASM 沙箱：限制线性内存（默认 64 MiB，可配置 `plugins.memory_limit_mb`），支持按上下文超时中断。
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
5. ~~**插件生命周期**：按需激活（lazy 默认，首次使用才加载）、preload 预热、空闲自动卸载与三态可见性。~~ ✅

## 内置示例插件

| 插件 | 目录 | 运行时 | 说明 |
|------|------|--------|------|
| 短信宝 | `examples/smsbao` | wasm | 通用 HTTP + MD5 签名 |
| 阿里云短信 | `examples/aliyun-sms` | wasm | RPC + HMAC-SHA1 签名，模板参数取 `params` |
| 腾讯云短信 | `examples/tencent-sms` | wasm | TC3-HMAC-SHA256 签名，JSON POST |
| Webhook | `examples/process-webhook` | process | JSON POST 转发，演示 net/http |

云厂商插件均使用 Go 标准库在插件内完成签名，宿主无需引入任何厂商依赖。运行
`make plugin-example` 会把全部示例构建到 `plugins/`，随后可在后台「短信渠道」选择。
