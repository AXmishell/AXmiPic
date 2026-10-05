# 套餐、优惠券、订单与工单

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/plans` | 启用的套餐（无需登录） |
| GET | `/admin/plans` | 全部套餐 |
| POST/PUT/DELETE | `/admin/plans[/{id}]` | 新建 / 修改 / 删除套餐 |
| GET | `/admin/coupons` | 优惠券列表 |
| POST/PUT/DELETE | `/admin/coupons[/{id}]` | 新建 / 修改 / 删除优惠券 |
| POST | `/coupons/validate` | 校验优惠券（`{"code":"…","amount_cents":1000}`） |
| GET/POST | `/orders` | 我的订单 / 下单（管理员可见全部订单） |
| POST | `/orders/{id}/pay` | 完成手动/模拟订单的支付 |
| GET/POST | `/tickets` | 我的工单 / 新建工单（管理员可见全部） |
| GET | `/tickets/{id}` | 工单详情（含消息） |
| POST | `/tickets/{id}/reply` | 回复工单 |
| PATCH | `/admin/tickets/{id}` | 更新工单状态 |

套餐字段：`price_cents`（价格，分）、`duration_days`（0 为永久）、`quota_mb`（0 为不限）、`role_group_id`（购买后应用的角色组，可空）、`active`、`sort_order`。优惠券的 `type` 取值为 `fixed`（金额，分）或 `percent`（百分比），并支持 `min_amount_cents` 门槛、`max_uses` 总量、`per_user_limit` 每用户限次与 `expires_at`。下单请求体为 `{"plan_id":"…","coupon_code":"…","provider":"manual"}`；支付成功（含免费订单）后自动应用套餐的配额与角色组。

当 `duration_days` 大于 0 时，购买会从当前到期时间（未到期时）或现在起顺延有效期，用户可通过 `plan_expires_at`（`GET /auth/me` 返回）查看；到期后服务会（最迟一小时内）自动把该用户回退到默认角色组与默认配额。`duration_days` 为 0 表示永久，会清除到期时间。
