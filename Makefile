.PHONY: build run codegen tidy

VERSION=$(shell git describe --tags --always --dirty)

ifneq ("$(wildcard .env)","")
    include .env
    export $(shell sed 's/=.*//' .env)
endif

tidy:
	go mod tidy

codegen:
	@mkdir -p build
	curl http://localhost:8000/openapi.json > build/openapi.json
	oapi-codegen -config oapi-config.yaml build/openapi.json

build: #codegen
	go build -ldflags="-X 'codeflower-runner/internal.Version=$(VERSION)'" -o build/codeflower cmd/cli/main.go
	./build/codeflower --version

