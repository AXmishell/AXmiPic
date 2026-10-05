# 开发

```bash
make build   # 构建后端
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
pnpm build      # 构建到 ../internal/webui/dist（供 Go 内嵌）
```

修改前端后需重新执行 `pnpm build`，并重新编译 Go 二进制，嵌入的资源才会更新。
