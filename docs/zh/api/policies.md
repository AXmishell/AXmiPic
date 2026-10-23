# 角色组与策略

角色组把一组带类型的策略绑定在一起并分配给普通用户，从而按角色控制资源与功能。每个类型在一个角色组内至多绑定一个策略，同一类型再次绑定时会替换旧策略。内置策略类型：

| 类型 | 作用 | 设置字段（JSON） |
|------|------|------------------|
| `quota` | 存储配额 | `quota_mb`（0 表示不限） |
| `upload` | 单文件大小与媒体类型 | `max_size_mb`、`allowed_mime_types` |
| `rate` | 上传与图片读取限流 | `upload_per_minute`、`upload_burst`、`image_per_minute`、`image_burst` |
| `processing` | 即时图片处理能力 | `enabled`、`max_width`、`max_height`、`default_quality`、`allowed_formats` |
| `feature` | 功能开关集合 | `features`（如 `plaza`、`albums`、`api_tokens`、`batch_upload`、`paste_upload`、`drag_upload`、`embed_code`、`share`、`share_password`） |

策略字段为可选覆盖值：未出现的字段沿用配置文件中的默认值。首次启动会依据 `auth`/`upload`/`processing`/`limits` 配置播种一个「默认角色组」及每个类型的一条策略；历史用户会自动归入默认组。

上传与图片读取的限流会在每次请求时按调用方所属角色组的 `rate` 策略解析执行（访客与匿名请求使用 Guest 组，回退默认组），管理员调整策略后最多约 30 秒生效；`limits.*` 配置仅作为策略默认值的种子。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/auth/policies` | 当前账户最终生效的策略集合 |
| GET/POST | `/admin/role-groups` | 角色组列表 / 新建 |
| GET/PUT/DELETE | `/admin/role-groups/{id}` | 角色组详情 / 修改 / 删除 |
| POST | `/admin/role-groups/{id}/policies` | 绑定策略（体为 `{"policy_id":"…"}`，同类型替换） |
| DELETE | `/admin/role-groups/{id}/policies/{policyID}` | 解除绑定 |
| GET/POST | `/admin/policies` | 策略列表（可带 `?type=`）/ 新建 |
| GET/PUT/DELETE | `/admin/policies/{id}` | 策略详情 / 修改 / 删除 |

通过 `PATCH /admin/customers/{id}`，请求体可携带 `"role_group_id"`（空字符串表示回退默认组）为普通用户分配角色组；分配时其存储配额会按角色组的 `quota` 策略同步。修改配额策略后，引用该策略的所有角色组的成员配额也会被重新同步。
