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
pnpm dev        # Vite 开发服务器（直接访问 :5173 时会代理 /api 与 /i 到 :8080）
pnpm typecheck  # vue-tsc 类型检查
```

后端默认使用编译时内嵌的前端产物。要在开发时获得 **HMR 热更新**，请在配置文件中开启热更新模式（默认关闭），并先启动 Vite：

```yaml
# configs/config.yaml
web:
  hot_reload:
    enabled: true
    target: "http://127.0.0.1:5173"
```

```bash
cd web && pnpm dev              # 终端 1：Vite
./bin/axmipic -config configs/config.yaml   # 终端 2：后端（默认 :8080）
```

开启后，后端会把页面、模块与 HMR WebSocket 反向代理到 Vite，访问
`http://localhost:8080` 即可边改边看，无需重新构建。`/api`、`/i/` 与
`/healthz` 始终由后端处理。该模式仅用于本地开发，生产环境请保持关闭
（或直接移除该配置），并执行 `make build` 内嵌最新前端产物。

构建前端请用 `make web`（会构建到 `../internal/webui/dist` 并补回占位文件
`.gitkeep`，供 Go 内嵌）；直接 `pnpm build` 会清空 `dist` 从而删除占位文件。
修改前端后重新执行 `make build`（或 `make web && make build-go`），嵌入的资源才会更新。
