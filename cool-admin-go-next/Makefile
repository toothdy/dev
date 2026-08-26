SHELL := /bin/sh

GOCACHE ?= $(CURDIR)/.cache/go-build
export GOCACHE

.PHONY: check check-build check-full check-architecture check-format check-mod check-vet test-integration test-race test-unit

check: check-mod check-format check-vet check-architecture test-unit check-build

check-mod:
	@echo "[check-mod] 校验 Go Module"
	@go mod tidy -diff
	@go mod verify

check-format:
	@echo "[check-format] 校验 Go 格式"
	@unformatted="$$(find . -type f -name '*.go' -not -path './.cache/*' -not -path './vendor/*' -exec gofmt -l {} +)"; \
	if [ -n "$$unformatted" ]; then \
		echo "以下文件未通过 gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

check-vet:
	@echo "[check-vet] 执行 go vet"
	@go vet ./...

check-architecture:
	@echo "[check-architecture] 校验依赖边界"
	@if [ -d test/architecture ]; then \
		go test ./test/architecture -count=1; \
	else \
		echo "  依赖边界测试尚未建立，跳过"; \
	fi

test-unit:
	@echo "[test-unit] 执行单元测试"
	@go test ./... -count=1

check-build:
	@echo "[check-build] 执行 Cool 静态检查与应用构建"
	@go run ./cmd/cool build

test-race:
	@echo "[test-race] 执行 Race Test"
	@CGO_ENABLED=1 go test -race ./... -count=1

test-integration:
	@echo "[test-integration] 执行三数据库集成测试"
	@test -x test/integration/run.sh || { echo "集成测试 Runner 尚未建立"; exit 1; }
	@test/integration/run.sh

check-full: check test-race test-integration
