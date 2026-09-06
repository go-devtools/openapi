# Require the exact development toolchain without automatic upgrades hiding the minimum version.
GO ?= go
export GOTOOLCHAIN = local

.PHONY: dev
# Run module tests and verify that all product packages build.
dev:
	@$(GO) version | grep -q '^go version go1.27.1 '
	$(GO) test ./...
	$(GO) build ./...
