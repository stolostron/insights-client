#!/bin/bash
GOLANGCI_LINT_VERSION="v1.47.1"
GOLANGCI_LINT_CACHE=/tmp/golangci-cache
GOOS=$(go env GOOS)
GOPATH=$(go env GOPATH)
export GOFLAGS=""
DOWNLOAD_URL="https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64.tar.gz"
curl -sfL "${DOWNLOAD_URL}" | tar -C "${GOPATH}/bin" -zx --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64/golangci-lint"
$(GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE} CGO_ENABLED=0 golangci-lint run)