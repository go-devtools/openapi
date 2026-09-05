# 开发入口要求精确工具链，禁止自动升级掩盖最低版本。
GO ?= go
export GOTOOLCHAIN = local

.PHONY: dev
# 执行当前模块测试并验证所有产品包可以构建。
dev:
	@$(GO) version | grep -q '^go version go1.27.1 '
	$(GO) test ./...
	$(GO) build ./...
