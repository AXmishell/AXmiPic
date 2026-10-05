.PHONY: build run vet tidy test test-sdk sdk clean docs docs-serve

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

# 运行 Go SDK 的检查与测试（独立模块）。
test-sdk:
	cd sdk/go && go vet ./... && go test ./...

# 构建并测试全部 SDK。
sdk: test-sdk
	cd sdk/typescript && pnpm install && pnpm typecheck && pnpm build && pnpm test

# 构建文档站（产物位于 docs/.vitepress/dist）。
docs:
	cd docs && pnpm install && pnpm build

# 本地预览文档站（热更新）。
docs-serve:
	cd docs && pnpm install && pnpm dev

clean:
	go clean
	rm -rf bin data sdk/typescript/dist
