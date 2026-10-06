.PHONY: build build-go web run vet tidy test test-sdk sdk clean docs docs-serve plugin-example test-plugin

BINARY := bin/axmipic

# 完整构建：先构建前端（产出到 internal/webui/dist），再编译内嵌它的后端二进制。
build: web
	go build -o $(BINARY) ./cmd/axmipic

# 仅构建后端：使用已存在的 internal/webui/dist（无前端产物时内嵌占位文件，
# 运行后网页返回 503，可在需要时再执行 `make web`）。适合无 Node 环境。
build-go:
	go build -o $(BINARY) ./cmd/axmipic

# 构建前端单页应用，产物输出到 internal/webui/dist。Vite 会清空 outDir，因此
# 构建后补回 .gitkeep 占位文件（保证仅构建后端时 go:embed 可编译且工作区干净）。
web:
	cd web && pnpm install --frozen-lockfile && pnpm build && touch ../internal/webui/dist/.gitkeep

# 本地运行：先构建前端，再运行。
run: web
	go run ./cmd/axmipic -config configs/config.example.yaml

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test ./...

# 运行插件框架测试（需要 Go WASM 工具链）。设置 AXMIPIC_SKIP_WASM_TESTS=1 可跳过
# WASM 端到端用例。
test-plugin:
	go test ./internal/plugin/

# 运行 Go SDK 的检查与测试（独立模块）。
test-sdk:
	cd sdk/go && go vet ./... && go test ./...

# 构建并测试全部 SDK。
sdk: test-sdk
	cd sdk/typescript && pnpm install && pnpm typecheck && pnpm build && pnpm test

# 构建示例插件（短信宝 / 阿里云 / 腾讯云 / 进程 Webhook）到 plugins/，供运行时加载。
plugin-example:
	@for name in smsbao aliyun-sms tencent-sms; do \
		mkdir -p plugins/$$name; \
		( cd sdk/plugin-go/examples/$$name && \
		  GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared \
		    -o $(CURDIR)/plugins/$$name/plugin.wasm . && \
		  cp plugin.yaml $(CURDIR)/plugins/$$name/plugin.yaml ) && \
		echo "built plugins/$$name"; \
	done
	@mkdir -p plugins/process-webhook; \
	( cd sdk/plugin-go/examples/process-webhook && \
	  go build -o $(CURDIR)/plugins/process-webhook/plugin-bin . && \
	  cp plugin.yaml $(CURDIR)/plugins/process-webhook/plugin.yaml ) && \
	echo "built plugins/process-webhook"

# 构建文档站（产物位于 docs/.vitepress/dist）。
docs:
	cd docs && pnpm install && pnpm build

# 本地预览文档站（热更新）。
docs-serve:
	cd docs && pnpm install && pnpm dev

clean:
	go clean
	rm -rf bin data sdk/typescript/dist
