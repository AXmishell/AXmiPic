# SDK

仓库提供两套覆盖全部 `/api/v1` 接口的客户端 SDK：

| 语言 | 位置 | 说明 |
|------|------|------|
| Go | `sdk/go` | 独立模块 `github.com/AXmishell/axmipic/sdk/go`，零第三方依赖 |
| TypeScript | `sdk/typescript` | 包 `@axmipic/sdk`，基于 `fetch`，支持浏览器与 Node 18+ |

Go SDK 示例：

```go
client, _ := axmipic.New("https://pic.example.com",
    axmipic.WithToken("会话或 API 令牌"))
image, err := client.Upload(ctx, "photo.png", data)
url := axmipic.TransformParams{Width: 400, Fit: "cover"}.TransformURL(image.URL)
```

TypeScript SDK 示例：

```ts
const client = new AxmipicClient({ baseUrl: 'https://pic.example.com' })
await client.login('alice', 'password123')
const image = await client.upload(file.name, file)
const url = transformUrl(image.url, { w: 400, fit: 'cover' })
```

两套 SDK 的方法一一对应，均覆盖认证、上传（含预签名直传）、图片、相册、分享、站点内容、套餐/订单/优惠券、工单与管理员接口，并把错误统一为 `*Error` / `AxmipicError`。详见各自的 `README.md`。
