# Copyright Dify Corp. 2025
# SPDX-License-Identifier: MPL-2.0

TEST?=$$(go list ./... | grep -v /vendor/)
HOSTNAME=registry.terraform.io
NAMESPACE=dify
NAME=dify
BINARY=terraform-provider-${NAME}
VERSION=0.1.0

.PHONY: build install test clean

build:
	go build -o ${BINARY}

install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$$(go env GOOS)_$$(go env GOARCH)
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$$(go env GOOS)_$$(go env GOARCH)/${BINARY}

test:
	go test -i ./... || exit 128
	go test -v ./... || exit 1

clean:
	rm -f ${BINARY}

# Development mode: run the provider locally with debug support
dev:
	go run main.go -debug
