.PHONY: build install clean test lint help

# 编译目标
build:
	@echo "正在编译 k8s-deployer..."
	@go build -o bin/k8s-deployer ./cmd/k8s-deployer
	@echo "✓ 编译完成: bin/k8s-deployer"

# 安装到系统
install:
	@echo "正在安装 k8s-deployer..."
	@go install cmd/k8s-deployer/main.go
	@echo "✓ 安装完成"

# 清理编译产物
clean:
	@echo "正在清理..."
	@rm -rf bin/
	@echo "✓ 清理完成"

# 运行测试
test:
	@echo "正在运行测试..."
	@go test -v ./...

# 代码检查
lint:
	@echo "正在进行代码检查..."
	@golangci-lint run

# 下载依赖
deps:
	@echo "正在下载依赖..."
	@go mod download
	@go mod tidy
	@echo "✓ 依赖下载完成"

# 显示帮助信息
help:
	@echo "K8s Deployer Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  make build    - 编译项目"
	@echo "  make install  - 安装到系统"
	@echo "  make clean    - 清理编译产物"
	@echo "  make test     - 运行测试"
	@echo "  make lint     - 代码检查"
	@echo "  make deps     - 下载依赖"
	@echo "  make help     - 显示此帮助信息"

