// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateServiceAPIEnabled(t *testing.T) {
	for name, value := range map[string]types.Bool{
		"null":    types.BoolNull(),
		"unknown": types.BoolUnknown(),
		"true":    types.BoolValue(true),
	} {
		var diags diag.Diagnostics
		validateServiceAPIEnabled(value, &diags)
		if diags.HasError() {
			t.Fatalf("%s: unexpected diagnostic %v", name, diags)
		}
	}

	var diags diag.Diagnostics
	validateServiceAPIEnabled(types.BoolValue(false), &diags)
	if !diags.HasError() {
		t.Fatal("false: expected a validation error")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Detail(), "Service API") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostic does not explain the Service API requirement: %v", diags)
	}
}

func TestSetPartialCreateState(t *testing.T) {
	data := AppResourceModel{
		ID:                types.StringValue("app-1"),
		Name:              types.StringUnknown(),
		Description:       types.StringUnknown(),
		Mode:              types.StringUnknown(),
		DSLYaml:           types.StringValue("kind: app"),
		ExportedDSLYaml:   types.StringUnknown(),
		DraftHash:         types.StringUnknown(),
		ServiceAPIEnabled: types.BoolUnknown(),
	}
	cfg := AppResourceModel{
		Name:              types.StringValue("configured"),
		ServiceAPIEnabled: types.BoolValue(true),
	}

	setPartialCreateState(&data, cfg)

	if data.ID.ValueString() != "app-1" {
		t.Fatalf("ID lost: %v", data.ID)
	}
	if data.Name.ValueString() != "configured" {
		t.Fatalf("configured name not preserved: %v", data.Name)
	}
	if !data.Description.IsNull() || !data.Mode.IsNull() ||
		!data.ExportedDSLYaml.IsNull() || !data.DraftHash.IsNull() {
		t.Fatalf("export-derived attributes must be null: %+v", data)
	}
	if data.ServiceAPIEnabled.ValueBool() != true {
		t.Fatalf("configured service_api_enabled not preserved: %v", data.ServiceAPIEnabled)
	}
	if data.DSLYaml.ValueString() != "kind: app" {
		t.Fatalf("dsl_yaml desired state changed: %v", data.DSLYaml)
	}

	// Unconfigured attributes stay null.
	cfg = AppResourceModel{}
	setPartialCreateState(&data, cfg)
	if !data.Name.IsNull() || !data.Description.IsNull() || !data.ServiceAPIEnabled.IsNull() {
		t.Fatalf("unconfigured attributes must be null: %+v", data)
	}
}
