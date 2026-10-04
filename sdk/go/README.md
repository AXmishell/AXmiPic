# AXmiPic Go SDK

AXmiPic 图床服务的 Go 客户端，覆盖 `/api/v1` 下的全部接口。

## 安装

```bash
go get github.com/AXmishell/axmipic/sdk/go
```

## 快速开始

```go
package main

import (
	"context"
	"log"
	"os"

	axmipic "github.com/AXmishell/axmipic/sdk/go"
)

func main() {
	ctx := context.Background()
	client, err := axmipic.New("https://pic.example.com")
	if err != nil {
		log.Fatal(err)
	}

	// 登录（成功后自动保存令牌），也可用 client.SetToken("<API 令牌>")。
	if _, err := client.Login(ctx, "alice", "password123"); err != nil {
		log.Fatal(err)
	}

	data, err := os.ReadFile("photo.png")
	if err != nil {
		log.Fatal(err)
	}
	image, err := client.Upload(ctx, "photo.png", data)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("uploaded %s -> %s", image.ID, image.URL)
}
```

## 支持的能力

| 分组 | 方法 |
|------|------|
| 认证 | `Register` `Login` `LoginAdmin` `Me` `Policies` `CreateToken` `ListTokens` `RevokeToken` |
| 上传 | `Upload` `Presign` `Confirm` `PutPresigned` |
| 图片 | `ListImages` `GetImage` `RenameImage` `DeleteImage` `BatchImages` `ListPlaza` `ListPublicAlbums` `GetPublicProfile` |
| 相册 | `ListAlbums` `CreateAlbum` `GetAlbum` `UpdateAlbum` `DeleteAlbum` `ListAlbumImages` |
| 分享 | `CreateShare` `ListShares` `RevokeShare` `ShareInfo` `AccessShare` |
| 站点 | `ListAnnouncements` `GetPage` `CreateReport` 及管理员公告/举报/页面接口 |
| 计费 | `ListPlans` `ValidateCoupon` `CreateOrder` `ListOrders` `PayOrder` 及管理员套餐/优惠券接口 |
| 工单 | `CreateTicket` `ListTickets` `GetTicket` `ReplyTicket` `AdminSetTicketStatus` |
| 管理 | `AdminStats`、用户、存储、角色组、策略、通知、安全与处理驱动等 |

## 图片处理 URL

`TransformParams` 可生成即时处理链接：

```go
url := axmipic.TransformParams{
	Width: 400, Fit: "cover", Format: "webp", Quality: 80,
	Watermark: "AXmiPic", WatermarkPosition: "bottom-right",
}.TransformURL(image.URL)
```

## 错误处理

所有非成功响应返回 `*axmipic.Error`：

```go
_, err := client.GetImage(ctx, "missing")
var apiErr *axmipic.Error
if errors.As(err, &apiErr) && apiErr.IsNotFound() {
	// 处理 404
}
```

## 测试

```bash
cd sdk/go
go test ./...
```
