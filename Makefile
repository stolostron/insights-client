# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

BINDIR ?= output

include build/Configfile

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

e2e-tests:
	@echo "Run e2e-tests"
	./build/run-e2e-tests.sh

