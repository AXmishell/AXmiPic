.PHONY: build run vet tidy test test-sdk sdk clean

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

clean:
	go clean
	rm -rf bin data sdk/typescript/dist
