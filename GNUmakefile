# Copyright Dify Corp. 2025
# SPDX-License-Identifier: MPL-2.0

TEST?=$$(go list ./... | grep -v /vendor/)
HOSTNAME=registry.terraform.io
NAMESPACE=wylswz
NAME=dify
BINARY=terraform-provider-${NAME}
VERSION=0.0.1
GOBIN_DIR ?= $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

default: fmt lint install generate

build:
	go build -o $(BINARY) .

install: build
	mkdir -p "$(GOBIN_DIR)"
	cp "$(BINARY)" "$(GOBIN_DIR)/$(BINARY)"

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
	terraform fmt -check -recursive ./internal/provider/testdata/acceptance
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate -provider-name dify

test:
	go test -v -cover -timeout=120s -parallel=10 ${TEST}

# Acceptance tests require a live Dify instance. They only run under TF_ACC=1
# and fail (never silently skip) without live DIFY_HOST / DIFY_TOKEN.
# scripts/dify-acceptance.sh boots a pinned upstream Dify and sets these.
testacc:
	TF_ACC=1 go test -v -count=1 -timeout=20m -run '^(TestAccAppLifecycle|TestAccTools|TestAccAppCreatePartialFailure|TestAccDatasetLifecycle)$$' ./internal/provider

# Full parity run of examples/complete: additionally needs TF_ACC_COMPLETE=1
# plus OPENAI_API_KEY / TAVILY_API_KEY (and optionally MCP_SERVER_URL).
testacc-complete:
	TF_ACC=1 TF_ACC_COMPLETE=1 go test -v -count=1 -timeout=30m -run '^TestAccComplete$$' ./internal/provider

clean:
	rm -f ${BINARY}

# Development mode: run the provider locally with debug support
dev:
	go run main.go -debug

.PHONY: build install install-local lint fmt generate test testacc testacc-complete clean dev
