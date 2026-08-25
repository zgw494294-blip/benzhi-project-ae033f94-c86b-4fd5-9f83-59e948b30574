# BENZHI_README

基于 Go 实现的stage-rigging-clearance Web 项目，一款后端服务，已完整实现剧院舞台吊挂换景安全验证与开演放行工作台，包含确定性载荷规则、有界排练、偏差整改、乐观并发、SQLite WAL 持久化、摘要链审计、不可变凭据、原生浏览器页面和真实 HTTP 自检。

## 项目说明
- 项目：benzhi-project-ae033f94-c86b-4fd5-9f83-59e948b30574
- 项目用途：已完整实现剧院舞台吊挂换景安全验证与开演放行工作台，包含确定性载荷规则、有界排练、偏差整改、乐观并发、SQLite WAL 持久化、摘要链审计、不可变凭据、原生浏览器页面和真实 HTTP 自检。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-ae033f94-c86b-4fd5-9f83-59e948b30574-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-ae033f94-c86b-4fd5-9f83-59e948b30574-arm64 linux/arm64
docker run -it benzhi-project-ae033f94-c86b-4fd5-9f83-59e948b30574-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19081`
