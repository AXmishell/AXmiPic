.PHONY: build run vet tidy test test-sdk sdk clean docs docs-serve plugin-example test-plugin

BINARY := bin/axmipic

build:
	go build -o $(BINARY) ./cmd/axmipic

run:
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
