# SDK

リポジトリには、すべての `/api/v1` エンドポイントをカバーする 2 つのクライアント SDK が用意されています:

| 言語 | 場所 | 備考 |
|------|------|------|
| Go | `sdk/go` | スタンドアロンモジュール `github.com/AXmishell/axmipic/sdk/go`。サードパーティ依存はゼロ |
| TypeScript | `sdk/typescript` | パッケージ `@axmipic/sdk`。`fetch` ベースで、ブラウザと Node 18 以上に対応 |

Go SDK の例:

```go
client, _ := axmipic.New("https://pic.example.com",
    axmipic.WithToken("session or API token"))
image, err := client.Upload(ctx, "photo.png", data)
url := axmipic.TransformParams{Width: 400, Fit: "cover"}.TransformURL(image.URL)
```

TypeScript SDK の例:

```ts
const client = new AxmipicClient({ baseUrl: 'https://pic.example.com' })
await client.login('alice', 'password123')
const image = await client.upload(file.name, file)
const url = transformUrl(image.url, { w: 400, fit: 'cover' })
```

2 つの SDK はメソッドが 1 対 1 で対応しており、どちらも認証、アップロード（署名付き直接アップロードを含む）、画像、アルバム、共有、サイトコンテンツ、プラン/注文/クーポン、チケット、管理エンドポイントをカバーしています。エラーは `*Error` / `AxmipicError` に正規化されます。詳細は各 `README.md` を参照してください。
