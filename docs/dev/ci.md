# 持续集成

仓库包含 GitHub Actions 工作流：

- `.github/workflows/ci.yml`：后端执行 `go mod tidy` 整洁性、`gofmt`、`go vet`、`staticcheck`、构建与竞态测试；前端执行类型检查与构建；并做前后端端到端构建。
- `.github/workflows/docker.yml`：构建多架构容器镜像并推送到 GHCR（Pull Request 仅构建、不推送）。
- `.github/workflows/release.yml`：推送 `v*` 标签（或手动指定标签）时交叉编译多平台二进制并创建 GitHub Release；发行说明由该标签相对上一个标签的提交信息生成（首次发版则取该标签提交的信息）。
