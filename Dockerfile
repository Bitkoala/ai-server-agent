# AI Server Agent - 多阶段构建 (精简优化版)
# Stage 1: 编译
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# 先复制依赖文件以利用 Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译纯静态二进制
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -trimpath \
    -o agent ./cmd/agent

# 生成 SBOM (Software Bill of Materials)
RUN go version -m agent > /build/sbom.txt

# Stage 2: 运行 (基于 alpine 最小镜像)
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app
COPY --from=builder /build/agent .
COPY --from=builder /build/sbom.txt .
COPY configs/ configs/

RUN mkdir -p /app/data \
    && chown -R 65534:65534 /app

EXPOSE 9090

# HEALTHCHECK — 每 30s 检查进程是否存活
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/agent", "-health"]

# 非 root 用户运行
USER 65534:65534

VOLUME ["/app/data"]

ENTRYPOINT ["/app/agent", "-config", "configs/config.yaml"]
