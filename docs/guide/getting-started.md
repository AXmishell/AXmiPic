# 快速开始

## 环境要求

- Go 1.26 或更高版本
- （可选，仅重新构建前端时需要）Node.js 22+ 与 pnpm 9

前端产物已内嵌进 `internal/webui/dist`，仅构建后端时无需 Node 环境。

## 构建与运行

```bash
# 构建后端二进制（前端已内嵌）
make build          # 产物位于 bin/axmipic

# 使用示例配置运行
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

或直接运行：

```bash
make run            # 等价于 go run ./cmd/axmipic -config configs/config.example.yaml
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
