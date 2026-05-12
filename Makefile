.PHONY: build

ifneq ("$(wildcard .env)","")
    include .env
    export $(shell sed 's/=.*//' .env)
endif

build:
	@mkdir -p build
	go build -o build/codeflower cmd/main.go

run:
	./build/codeflower $(ARGS)

