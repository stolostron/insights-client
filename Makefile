# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

BINDIR ?= output

include build/Configfile

USE_VENDORIZED_BUILD_HARNESS ?=

ifndef USE_VENDORIZED_BUILD_HARNESS
-include $(shell curl -s -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)
else
-include vbh/.build-harness-vendorized
endif

default::
	@echo "Build Harness Bootstrapped"

.PHONY: deps
deps:
	 go mod tidy

.PHONY: build
build:
	 CGO_ENABLED=0 go build -o $(BINDIR)/insights-client ./

.PHONY: build-linux
build-linux:
	make build GOOS=linux

.PHONY: lint
lint:
	 go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.38.0
	 golangci-lint run --timeout=2m

run:
	 go run main.go

.PHONY: test
test:
	 go test ./... -v -coverprofile cover.out

.PHONY: coverage
coverage:
	 go tool cover -html=cover.out -o=cover.html


.PHONY: clean
clean::
	go clean
	rm -f cover*
	rm -rf ./$(BINDIR)

# Build the docker image
docker-build: 
	docker build -f Dockerfile . -t $(shell cat COMPONENT_NAME)
