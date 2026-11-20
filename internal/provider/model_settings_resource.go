// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ModelSettingsResource{}

func NewModelSettingsResource() resource.Resource {
	return &ModelSettingsResource{}
}

// ModelSettingsResource defines the resource implementation.
type ModelSettingsResource struct {
	client *DifyClient
}

// ModelSettingsEntryModel describes one load-balancing entry.
type ModelSettingsEntryModel struct {
	CredentialID types.String `tfsdk:"credential_id"`
	Enabled      types.Bool   `tfsdk:"enabled"`
}

// ModelSettingsResourceModel describes the resource data model.
type ModelSettingsResourceModel struct {
	ProviderName         types.String              `tfsdk:"provider_name"`
	Model                types.String              `tfsdk:"model"`
	ModelType            types.String              `tfsdk:"model_type"`
	ConfigFrom           types.String              `tfsdk:"config_from"`
	Enabled              types.Bool                `tfsdk:"enabled"`
	LoadBalancingEnabled types.Bool                `tfsdk:"load_balancing_enabled"`
	LoadBalancingEntries []ModelSettingsEntryModel `tfsdk:"load_balancing_entries"`
}

func (r *ModelSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_model_settings"
}

func (r *ModelSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one model's enablement and load-balancing settings. " +
			"Load balancing is declarative: entries stored on the server but not listed in " +
			"`load_balancing_entries` are deleted. One entry with `credential_id` unset (null) " +
			"reuses the model's or provider's active credential.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Model provider id (e.g. `langgenius/openai/openai`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"model": schema.StringAttribute{
				MarkdownDescription: "Model name.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"model_type": schema.StringAttribute{
				MarkdownDescription: "Model type (`llm`, `text-embedding`, `rerank`, `speech2text`, `tts`, `moderation`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config_from": schema.StringAttribute{
				MarkdownDescription: "Which credential table load-balancing entries point at: " +
					"`predefined-model` (provider credentials) or `custom-model` (the model's own).",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the model is enabled for this workspace.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"load_balancing_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether load balancing is on for this model.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"load_balancing_entries": schema.ListNestedAttribute{
				MarkdownDescription: "Desired load-balancing entries, in order. `credential_id` references " +
					"a saved credential; null names the entry that reuses the active credential (at most one).",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"credential_id": schema.StringAttribute{
							MarkdownDescription: "Saved credential id; null for the entry reusing the active credential.",
							Optional:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether this entry participates in balancing.",
							Required:            true,
						},
					},
				},
			},
		},
	}
}

func (r *ModelSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*DifyClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *DifyClient, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// entryRequests converts the configured entries to the API request shape.
func entryRequests(entries []ModelSettingsEntryModel) []LoadBalancingEntryRequest {
	out := make([]LoadBalancingEntryRequest, 0, len(entries))
	for _, e := range entries {
		out = append(out, LoadBalancingEntryRequest{
			CredentialID: e.CredentialID.ValueStringPointer(),
			Enabled:      e.Enabled.ValueBool(),
		})
	}
	return out
}

func (r *ModelSettingsResource) put(ctx context.Context, data *ModelSettingsResourceModel) error {
	_, err := r.client.SetModelSettings(
		ctx,
		data.ProviderName.ValueString(),
		data.Model.ValueString(),
		data.ModelType.ValueString(),
		data.ConfigFrom.ValueString(),
		data.Enabled.ValueBool(),
		data.LoadBalancingEnabled.ValueBool(),
		entryRequests(data.LoadBalancingEntries),
	)
	return err
}

func (r *ModelSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ModelSettingsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.put(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set model settings: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ModelSettingsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := r.client.GetModelSettings(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), data.ConfigFrom.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read model settings: %s", err))
		return
	}

	data.Enabled = types.BoolValue(settings.Enabled)
	data.LoadBalancingEnabled = types.BoolValue(settings.LoadBalancing.Enabled)

	// The server synthesizes an __inherit__ row whenever provider credentials
	// exist, so reads always report at least that entry. Only a configured
	// null credential_id declares it; an undeclared inherit row is filtered out
	// or it would show as a perpetual diff.
	declaredInherit := false
	for _, e := range data.LoadBalancingEntries {
		if e.CredentialID.IsNull() {
			declaredInherit = true
		}
	}
	entries := make([]ModelSettingsEntryModel, 0, len(settings.LoadBalancing.Entries))
	for _, e := range settings.LoadBalancing.Entries {
		if e.Inherit && !declaredInherit {
			continue
		}
		entries = append(entries, ModelSettingsEntryModel{
			CredentialID: types.StringPointerValue(e.CredentialID),
			Enabled:      types.BoolValue(e.Enabled),
		})
	}
	if len(entries) > 0 || data.LoadBalancingEntries != nil {
		data.LoadBalancingEntries = entries
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ModelSettingsResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.put(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set model settings: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ModelSettingsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reset to a plain enabled model with load balancing off and no entries.
	_, err := r.client.SetModelSettings(
		ctx,
		data.ProviderName.ValueString(),
		data.Model.ValueString(),
		data.ModelType.ValueString(),
		data.ConfigFrom.ValueString(),
		true,
		false,
		[]LoadBalancingEntryRequest{},
	)
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to reset model settings: %s", err))
		return
	}
}
