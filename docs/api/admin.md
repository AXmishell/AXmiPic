# 管理接口（需管理员）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/stats` | 实例统计 |
| GET | `/admin/customers` | 普通用户列表 |
| GET/POST | `/admin/admins` | 管理员列表 / 新建管理员 |
| PATCH/DELETE | `/admin/customers/{id}` | 启用/禁用、删除普通用户 |
| PATCH/DELETE | `/admin/admins/{id}` | 启用/禁用、删除管理员 |
| GET/POST | `/admin/storage` | 存储后端列表 / 新建 |
| GET/PUT/DELETE | `/admin/storage/{id}` | 存储后端详情 / 修改 / 删除 |
| POST | `/admin/storage/{id}/activate` | 切换默认存储（热切换） |

存储后端的密钥（S3/七牛的 Secret）以密文入库，接口仅返回「是否已设置」，不回传明文。

## 普通用户列表分页与筛选

`GET /admin/customers` 在不带任何查询参数时保持旧行为，返回全量用户数组（Go/TypeScript SDK 依赖该形状）。提供以下任一参数时返回分页对象：

| 参数 | 说明 |
|------|------|
| `page` | 页码，从 1 开始（默认 1） |
| `page_size` | 每页数量（默认 20，最大 200） |
| `keyword` | 按用户名或邮箱模糊匹配 |
| `status` | `enabled` / `disabled`，空表示全部 |
| `role_group_id` | 按角色组过滤 |
| `sort` | `newest`（默认）/ `oldest` / `username` / `usage` |

分页响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [ /* User */ ],
    "total": 100,
    "page": 1,
    "page_size": 20
  }
}
```
