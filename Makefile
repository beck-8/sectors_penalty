SHELL=/usr/bin/env bash
.PHONY: build 

BINARY="sectors_penalty"

build:
	go build -ldflags "-s -w" -o ${BINARY}

