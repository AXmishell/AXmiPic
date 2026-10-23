# SDK

The repository provides two client SDKs that cover every `/api/v1` endpoint:

| Language | Location | Notes |
|------|------|------|
| Go | `sdk/go` | Standalone module `github.com/AXmishell/axmipic/sdk/go`, with zero third-party dependencies |
| TypeScript | `sdk/typescript` | Package `@axmipic/sdk`, built on `fetch`, supports browsers and Node 18+ |

Go SDK example:

```go
client, _ := axmipic.New("https://pic.example.com",
    axmipic.WithToken("session or API token"))
image, err := client.Upload(ctx, "photo.png", data)
url := axmipic.TransformParams{Width: 400, Fit: "cover"}.TransformURL(image.URL)
```

TypeScript SDK example:

```ts
const client = new AxmipicClient({ baseUrl: 'https://pic.example.com' })
await client.login('alice', 'password123')
const image = await client.upload(file.name, file)
const url = transformUrl(image.url, { w: 400, fit: 'cover' })
```

The two SDKs have one-to-one method parity and both cover authentication, uploads (including presigned direct uploads), images, albums, shares, site content, plans/orders/coupons, tickets, and admin endpoints, with errors normalized to `*Error` / `AxmipicError`. See each `README.md` for details.
