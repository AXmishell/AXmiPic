# 开发

```bash
make build    # 完整构建：前端 + 后端二进制
make build-go # 仅构建后端（复用已有 internal/webui/dist）
make web      # 仅构建前端到 internal/webui/dist
make run      # 先构建前端，再运行
make vet     # go vet
make test    # go test ./...
make tidy    # go mod tidy
make test-sdk # 运行 Go SDK 测试
make sdk     # 构建并测试全部 SDK
make clean   # 清理 bin/ 与 data/
```

## 前端开发

```bash
cd web
pnpm install
pnpm dev        # 本地开发服务器（自动代理 /api 与 /i 到 :8080）
pnpm typecheck  # vue-tsc 类型检查
```

构建前端请用 `make web`（会构建到 `../internal/webui/dist` 并补回占位文件
`.gitkeep`，供 Go 内嵌）；直接 `pnpm build` 会清空 `dist` 从而删除占位文件。
修改前端后重新执行 `make build`（或 `make web && make build-go`），嵌入的资源才会更新。
