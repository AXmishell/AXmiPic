# 插件系统

AXmiPic 的插件分为**内置插件**与**外置插件**：

| 类型 | 运行方式 | 信任级别 | 适用场景 |
|------|----------|----------|----------|
| 内置（builtin） | 随宿主编译，进程内 Go Provider | 默认受信，**无沙箱**，常驻内存 | 资源占用低、稳定的基础渠道（日志短信、HTTP 短信、SMTP 邮件） |
| 外置（external） | 从 `plugins.dir` 加载，wazero WASM 沙箱 | 不可信，**必须沙箱**，默认懒加载 | 第三方短信服务商、后续可扩展的支付/存储渠道 |

内置插件不读取 `plugin.yaml`、不占用 WASM 运行时内存，也不能被卸载、替换或暂停；
外置插件只能使用 `runtime: wasm`，清单声明 `process` 或其它运行时会直接拒绝。

两种插件统一使用 `category + descriptor` 契约与同一套配置/启用/调用接口。

## 目录结构

```
plugins/                   插件根目录（默认，可配置 plugins.dir）
  smsbao/
    plugin.yaml            清单
    plugin.wasm            编译产物
internal/plugin/           宿主框架（加载、注册、沙箱、宿主能力）
sdk/plugin-go/             Go 插件 SDK
  examples/smsbao/         示例插件（短信宝，wasm）
  examples/aliyun-sms/     示例插件（阿里云短信，wasm）
  examples/tencent-sms/    示例插件（腾讯云短信，wasm）
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

启动时先注册内置插件（始终 enabled + loaded），再扫描 `plugins.dir` 加载外置
插件，逐个校验清单与校验和。外置插件默认采用 **lazy** 模式：只登记而不实例化，
首次使用时才真正加载；单个插件失败不会阻止其它插件或服务启动。

## 插件生命周期与内存管理

宿主把插件的「**逻辑启用**」与「**物理加载**」分开，由此定义三态（对外字段
`state`）：

| `state` | 逻辑启用 | 物理加载 | 含义 |
|---------|:---:|:---:|------|
| `disabled` | ❌ | ❌ | 已暂停（后台关闭）；不加载、不占内存 |
| `standby` | ✅ | ❌ | 已启用但**待激活**：未实例化，首次调用才加载 |
| `active` | ✅ | ✅ | 运行中：已实例化并占用内存 |

内置插件注册后始终为 `enabled + loaded`，不参与暂停与空闲卸载；下文的启用/暂停与
按需激活规则针对外置插件。

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
   调用的磁盘插件会被卸载（回到 `standby`，**保持启用**）；再次调用会透明重载并
   自动重放已保存配置。内置插件常驻，不会被空闲卸载。
5. **描述缓存**：卸载/暂停后，插件的 `descriptor`（字段、标题）缓存于 `settings`
   的 `plugin.desc.<name>`，后台表单与渠道列表仍可正常展示，不会因未加载而丢字段。
6. **关闭**：进程退出时关闭全部已加载实例。

并发安全：`Close` 与在途的 `Invoke`/`Configure` 通过互斥锁串行化；同一插件的首次
加载也在锁内双检，保证「调用进行中暂停」或并发首次调用不会重复实例化或并发关闭
wazero runtime。激活失败时插件保持 `standby` 并记录 `last_error`，不会误改启用意图。

内存与状态可见性：

- `GET /api/v1/admin/plugins/installed` 返回每个插件的 `state`（disabled/standby/active）、
  `builtin`、`sandboxed`、`enabled`、`loaded`、`memory_bytes`（已加载 WASM 插件的线性
  内存字节数；内置插件为 0）、`last_used`（Unix 秒，0 表示加载后尚未调用）与 `last_error`。
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
| GET | `/api/v1/admin/plugins/active?category=` | 读取某通知类别当前使用的插件 |
| PUT | `/api/v1/admin/plugins/active` | 设置当前渠道：`{"category":"notify.sms","name":"builtin-sms-http"}` |
| GET | `/api/v1/admin/plugins/registry` | 拉取插件市场索引 |
| POST | `/api/v1/admin/plugins/install` | 安装插件（JSON 按 URL/索引名，或原始 body 为归档） |
| GET | `/api/v1/admin/plugins/{name}` | 读取插件配置；秘钥字段仅返回是否已设置 |
| PUT | `/api/v1/admin/plugins/{name}/config` | 保存配置并即时应用（秘钥以密文存储） |
| PUT | `/api/v1/admin/plugins/{name}/enabled` | 启用/暂停插件（`{"enabled":bool}`，状态持久化） |
| POST | `/api/v1/admin/plugins/{name}/test` | 用当前配置发送一条测试通知 |
| POST | `/api/v1/admin/plugins/{name}/reload` | 重新加载插件（保持启用状态） |
| DELETE | `/api/v1/admin/plugins/{name}` | 卸载并删除外置插件（内置插件不可删除） |

后台「插件市场」是插件配置的**唯一入口**：按插件的 `fields` 渲染动态表单，普通字段直接展示，
`secret` 字段以密码框呈现且不回显；保存后可用「测试发送」在切换正式渠道前验证。
内置插件同样在此配置，但只能编辑配置，不能卸载、暂停或重新加载。

## 通知渠道（内置 + 外置）

短信与邮件都通过插件发送，启动时注册以下内置插件：

| 插件名 | 类别 | 说明 | 关键字段 |
|--------|------|------|----------|
| `builtin-sms-log` | `notify.sms` | 日志短信，调试用 | 无 |
| `builtin-sms-http` | `notify.sms` | 通用 HTTP 短信网关 | `provider`、`endpoint`、`method` |
| `builtin-email-log` | `notify.email` | 日志邮件，调试用 | 无 |
| `builtin-email-smtp` | `notify.email` | SMTP 邮件（STARTTLS / 隐式 TLS） | `host`、`port`、`username`、`password`（密文）、`from`、`use_tls` |

外置短信插件（如 `smsbao`）与内置插件共用同一套安装、配置、测试和调用流程。每个
通知类别通过 `plugin.active.<category>` 记录**当前渠道**：

- `PUT /api/v1/admin/plugins/active` 设置当前渠道，保存后即时生效；
- 未显式设置时回退到 `builtin-sms-log` / `builtin-email-log`；
- 当前渠道被暂停或卸载时，发送路径自动回退到内置日志渠道并记录告警；
- 宿主会用已保存的插件配置 `Configure` 实例，并把 `notify.Sender` 桥接到插件的
  `invoke("send", ...)`，载荷为中性消息（`to`/`subject`/`body`/`template`/
  `params`/`sign_name`），由插件映射到具体服务商。

> **从旧版本迁移**：首次启动时会把 `settings.smtp` 与旧的 `settings.sms` 自动迁移为
> `builtin-email-smtp` / `builtin-sms-http` 的插件配置，并设置当前渠道；迁移只执行
> 一次（`plugin.migrated.notify_v1`）。旧的通知设置页已移除，请统一到「插件市场」。

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
内存**，但保留文件与配置。状态持久化，重启后仍生效。暂停的插件不能设为当前通知
渠道，但其描述与配置仍可查看/编辑（来自 `plugin.desc.<name>` 缓存）。已启用
插件的空闲自动卸载见上文「插件生命周期与内存管理」。

宿主会校验大小、sha256 与 Ed25519 签名（配置了 `trusted_keys` 或
`require_signature` 时强制），随后安全解压（拒绝路径穿越、符号链接与超限内容），
校验清单并热加载。

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
- 外置插件只允许 WASM 运行时，统一运行在 wazero 沙箱中。
- 内置插件随宿主编译发布，默认受信、进程内运行；外置插件无法伪造内置身份
  （`builtin-` 名称前缀与 `runtime: builtin` 均被拒绝）。
- 秘钥字段由宿主以 AES-256-GCM 加密后写入 `settings` 表，AAD 绑定插件名与字段名，
  接口返回值绝不包含明文。
- `checksum` 可选校验，防止篡改入口文件。

## 构建与测试

```bash
make plugin-example   # 构建全部 WASM 示例插件到 plugins/
make test-plugin      # 运行插件框架测试（含 WASM 端到端）
```

## 路线图

1. ~~**M2**：把插件接入 `notify`、管理端动态表单与凭据加密、测试发送。~~ ✅
2. ~~**M3**：阿里云 / 腾讯云短信插件。~~ ✅（见 `sdk/plugin-go/examples/`）
3. ~~**M5**：在线安装、Ed25519 签名校验与插件市场索引。~~ ✅
4. ~~**插件生命周期**：按需激活（lazy 默认，首次使用才加载）、preload 预热、空闲自动卸载与三态可见性。~~ ✅
5. ~~**内置/外置分离**：内置通知插件（短信/邮件）免沙箱，外置插件强制 WASM 沙箱。~~ ✅

## 内置示例插件

| 插件 | 目录 | 运行时 | 说明 |
|------|------|--------|------|
| 短信宝 | `examples/smsbao` | wasm | 通用 HTTP + MD5 签名 |
| 阿里云短信 | `examples/aliyun-sms` | wasm | RPC + HMAC-SHA1 签名，模板参数取 `params` |
| 腾讯云短信 | `examples/tencent-sms` | wasm | TC3-HMAC-SHA256 签名，JSON POST |

云厂商插件均使用 Go 标准库在插件内完成签名，宿主无需引入任何厂商依赖。运行
`make plugin-example` 会把全部示例构建到 `plugins/`，随后可在后台「插件市场」配置并设为当前短信渠道。
