# 快速开始

## 环境要求

- Go 1.26 或更高版本
- Node.js 22+ 与 pnpm 9（构建前端需要；`make build` 会自动构建前端）

前端产物不提交到仓库，而是在构建时生成到 `internal/webui/dist`。仅构建后端（无 Node）时可复用已有的前端产物并执行 `make build-go`。

## 构建与运行

```bash
# 完整构建（前端 + 后端）
make build          # 产物位于 bin/axmipic

# 仅构建后端（复用已存在的 internal/webui/dist）
# make build-go

# 仅构建前端
# make web

# 使用示例配置运行
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

或直接运行：

```bash
make run            # 先构建前端，再运行
```

默认监听 `0.0.0.0:8080`，访问 `http://localhost:8080` 打开控制台。

## 创建管理员

有两种方式：

1. **配置文件播种**（推荐）：在 `configs/config.yaml` 中设置 `auth.bootstrap_admin: "用户名:密码"`，仅当 admins 表为空时生效：

   ```yaml
   auth:
     bootstrap_admin: "admin:你的强密码"
   ```

2. **后台创建**：以已有管理员登录后，在「用户管理 → 管理员」标签页新建管理员。

> 首次部署建议先配置 `auth.bootstrap_admin` 再启动，避免无管理员可用。注册入口只创建普通用户。
