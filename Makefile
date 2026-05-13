.PHONY: build run codegen tidy

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

build: codegen
	go build -o build/codeflower cmd/main.go
	./build/codeflower --version

