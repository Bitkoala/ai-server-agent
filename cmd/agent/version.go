package main

// 构建时通过 ldflags 注入
// go build -ldflags="-X main.Version=1.0.0 -X main.Commit=$(git rev-parse --short HEAD) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
