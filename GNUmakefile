# Copyright Dify Corp. 2025
# SPDX-License-Identifier: MPL-2.0

TEST?=$$(go list ./... | grep -v /vendor/)
HOSTNAME=registry.terraform.io
NAMESPACE=dify
NAME=dify
BINARY=terraform-provider-${NAME}
VERSION=0.1.0

default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

# Install into the local Terraform plugin mirror for dev overrides.
install-local: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$$(go env GOOS)_$$(go env GOARCH)
	cp ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/$$(go env GOOS)_$$(go env GOARCH)/${BINARY}

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w -e .
	terraform fmt -recursive ./examples/

generate:
	terraform fmt -recursive ./examples/
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate -provider-name dify

test:
	go test -v -cover -timeout=120s -parallel=10 ${TEST}

# Acceptance tests require a live Dify instance. Set TF_ACC=1 plus
# DIFY_HOST / DIFY_TOKEN (and optionally DIFY_WORKSPACE_ID).
testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ${TEST}

clean:
	rm -f ${BINARY}

# Development mode: run the provider locally with debug support
dev:
	go run main.go -debug

.PHONY: build install install-local lint fmt generate test testacc clean dev
