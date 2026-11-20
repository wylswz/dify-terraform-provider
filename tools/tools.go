// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

//go:build tools

// Package tools pins build-time tool dependencies in go.mod.
// Run `make generate` to regenerate docs/ via tfplugindocs.
package tools

import (
	// Documentation generation
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
