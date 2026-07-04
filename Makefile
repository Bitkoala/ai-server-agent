# AI Server Agent

VERSION ?= 1.0.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

# ============ 构建 ============
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/agent ./cmd/agent

# 构建 Docker 镜像
docker-build:
	docker build -t ai-server-agent:latest .

# ============ 运行 ============
run:
	go run -ldflags="$(LDFLAGS)" ./cmd/agent

# 运行测试
test:
	go test ./internal/... -v

# 测试覆盖率
test-cover:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out

# 构建所有平台二进制
build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/ai-server-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/ai-server-agent-linux-arm64 ./cmd/agent
	@echo "构建完成:"
	@ls -lh bin/

# 生成部署包（含二进制 + 配置 + 安装脚本）
release: build-all
	mkdir -p release
	cp bin/ai-server-agent-linux-amd64 release/
	cp bin/ai-server-agent-linux-arm64 release/
	cp configs/config.yaml release/
	cp scripts/install-remote.sh release/install.sh
	cd release && tar czf ../ai-server-agent-release.tar.gz .
	@echo "部署包: ai-server-agent-release.tar.gz"

# ============ 部署 ============
install:
	bash scripts/install.sh

# 一键远程安装 (需先设置 AGENT_DOWNLOAD_URL 环境变量)
install-remote:
	curl -fsSL $(AGENT_DOWNLOAD_URL)/install.sh | bash

# Docker Compose 部署
up:
	docker-compose up -d

down:
	docker-compose down

logs:
	docker-compose logs -f agent

# ============ 开发 ============
fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# 初始化数据目录
init:
	mkdir -p data

# 清理
clean:
	rm -rf bin/ data/
