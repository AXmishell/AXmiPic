# AXmiPic TypeScript SDK

AXmiPic 图床服务的 TypeScript / JavaScript 客户端，覆盖 `/api/v1` 下的全部接口。

## 安装

```bash
npm install @axmipic/sdk
# 或在仓库内直接构建
cd sdk/typescript && pnpm install && pnpm build
```

## 快速开始

```ts
import { AxmipicClient, transformUrl } from '@axmipic/sdk'

const client = new AxmipicClient({ baseUrl: 'https://pic.example.com' })

// 登录（成功后自动保存令牌），也可传入 token 选项或用 setToken。
await client.login('alice', 'password123')

// 浏览器：上传 File / Blob。
const input = document.querySelector<HTMLInputElement>('#file')!
const file = input.files![0]
const image = await client.upload(file.name, file)
console.log(image.url)

// 生成即时处理链接。
const processed = transformUrl(image.url, { w: 400, fit: 'cover', f: 'webp', wm: 'AXmiPic' })
```

## Node 环境

Node 18+ 内置了 `fetch`、`Blob` 与 `FormData`，可直接使用；也可注入自定义实现：

```ts
const client = new AxmipicClient({ baseUrl: 'https://pic.example.com', fetch: myFetch })
```

## 支持的能力

| 分组 | 方法 |
|------|------|
| 认证 | `register` `login` `loginAdmin` `me` `policies` `createToken` `listTokens` `revokeToken` |
| 上传 | `upload` `presign` `confirm` |
| 图片 | `listImages` `getImage` `renameImage` `deleteImage` `batchImages` `listPlaza` `listPublicAlbums` `getPublicProfile` |
| 相册 | `listAlbums` `createAlbum` `getAlbum` `updateAlbum` `deleteAlbum` `listAlbumImages` |
| 分享 | `createShare` `listShares` `revokeShare` `shareInfo` `accessShare` |
| 站点 | `listAnnouncements` `getPage` `createReport` 及管理员公告/举报/页面接口 |
| 计费 | `listPlans` `validateCoupon` `createOrder` `listOrders` `payOrder` 及管理员套餐/优惠券接口 |
| 工单 | `createTicket` `listTickets` `getTicket` `replyTicket` `adminSetTicketStatus` |
| 管理 | `adminStats`、用户、存储、角色组、策略、通知、安全与处理驱动等 |

## 错误处理

非成功响应会抛出 `AxmipicError`：

```ts
import { AxmipicError } from '@axmipic/sdk'

try {
  await client.getImage('missing')
} catch (error) {
  if (error instanceof AxmipicError && error.isNotFound()) {
    // 处理 404
  }
}
```

## 开发

```bash
cd sdk/typescript
pnpm install
pnpm typecheck
pnpm build
pnpm test
```
