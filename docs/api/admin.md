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
