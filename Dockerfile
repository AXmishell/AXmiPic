# ---------------------------------------------------------------------------
# 前端构建：在构建平台上执行一次，产物与目标架构无关。
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@9 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# 输出到 /src/internal/webui/dist（见 web/vite.config.ts 的 outDir）。
RUN pnpm build

# ---------------------------------------------------------------------------
# 后端构建：纯 Go 静态二进制（CGO 关闭），并按目标架构交叉编译。
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 用刚构建的前端产物填充 internal/webui/dist（该产物不提交到仓库）。
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN mkdir -p /out/data \
 && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /out/axmipic ./cmd/axmipic

# ---------------------------------------------------------------------------
# 运行：最小化的 distroless 静态镜像，以非 root 用户运行。
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/axmipic /usr/local/bin/axmipic
COPY --from=build --chown=65532:65532 /src/configs/config.example.yaml /app/configs/config.yaml
# 数据目录（SQLite、本地上传、自动生成的主密钥）以卷方式持久化。
COPY --from=build --chown=65532:65532 /out/data /app/data
ENV AXMIPIC_SERVER_HOST=0.0.0.0 \
    AXMIPIC_SERVER_PORT=8080
EXPOSE 8080
VOLUME ["/app/data"]
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/axmipic"]
CMD ["-config", "/app/configs/config.yaml"]
