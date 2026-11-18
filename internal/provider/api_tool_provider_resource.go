// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &APIToolProviderResource{}
var _ resource.ResourceWithImportState = &APIToolProviderResource{}

func NewAPIToolProviderResource() resource.Resource {
	return &APIToolProviderResource{}
}

// APIToolProviderResource defines the resource implementation.
type APIToolProviderResource struct {
	client *DifyClient
}

// APIToolProviderIconModel describes the provider's emoji tile.
type APIToolProviderIconModel struct {
	Content    types.String `tfsdk:"content"`
	Background types.String `tfsdk:"background"`
}

// APIToolProviderResourceModel describes the resource data model.
type APIToolProviderResourceModel struct {
	Name             types.String              `tfsdk:"name"`
	Icon             *APIToolProviderIconModel `tfsdk:"icon"`
	SchemaType       types.String              `tfsdk:"schema_type"`
	Schema           types.String              `tfsdk:"schema"`
	Credentials      types.Map                 `tfsdk:"credentials"`
	PrivacyPolicy    types.String              `tfsdk:"privacy_policy"`
	CustomDisclaimer types.String              `tfsdk:"custom_disclaimer"`
	Labels           []types.String            `tfsdk:"labels"`
	Tools            []types.String            `tfsdk:"tools"`
}

func (r *APIToolProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_tool_provider"
}

func (r *APIToolProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an API tool provider — an OpenAPI-schema-backed custom tool " +
			"provider keyed by name. Changing `name` renames the provider in place. " +
			"Credential values are write-only: describe returns them masked, so state keeps " +
			"the configured values. Import by name: `terraform import dify_api_tool_provider.x <name>`.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Provider name; changing it renames the provider in place.",
				Required:            true,
			},
			"icon": schema.SingleNestedAttribute{
				MarkdownDescription: "Emoji tile the workspace shows for the provider.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"content": schema.StringAttribute{
						MarkdownDescription: "Emoji content (e.g. `🌤`).",
						Required:            true,
					},
					"background": schema.StringAttribute{
						MarkdownDescription: "Tile background color (e.g. `#252525`).",
						Required:            true,
					},
				},
			},
			"schema_type": schema.StringAttribute{
				MarkdownDescription: "Schema type (`openapi`, `swagger`, `openai_plugin`, `openai_actions`).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("openapi"),
			},
			"schema": schema.StringAttribute{
				MarkdownDescription: "The schema text (an OpenAPI document) that defines the provider's tools.",
				Required:            true,
			},
			"credentials": schema.MapAttribute{
				MarkdownDescription: "Auth configuration the provider's calls send (e.g. `{auth_type = \"none\"}`). " +
					"Write-only and sensitive; state keeps the configured values.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Default: mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{
					"auth_type": types.StringValue("none"),
				})),
			},
			"privacy_policy": schema.StringAttribute{
				MarkdownDescription: "Privacy policy URL.",
				Optional:            true,
			},
			"custom_disclaimer": schema.StringAttribute{
				MarkdownDescription: "Custom disclaimer shown to users of the provider's tools.",
				Optional:            true,
			},
			"labels": schema.ListAttribute{
				MarkdownDescription: "Tool labels for the provider.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"tools": schema.ListAttribute{
				MarkdownDescription: "Names of the tools the schema defines.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (r *APIToolProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// request converts the model to the API payload.
func (m *APIToolProviderResourceModel) request() apiToolProviderRequest {
	req := apiToolProviderRequest{
		Name:             m.Name.ValueString(),
		Credentials:      credentialsToMap(m.Credentials),
		SchemaType:       m.SchemaType.ValueString(),
		Schema:           m.Schema.ValueString(),
		PrivacyPolicy:    m.PrivacyPolicy.ValueStringPointer(),
		CustomDisclaimer: m.CustomDisclaimer.ValueStringPointer(),
		Labels:           stringValues(m.Labels),
	}
	if m.Icon != nil {
		req.Icon = map[string]string{
			"content":    m.Icon.Content.ValueString(),
			"background": m.Icon.Background.ValueString(),
		}
	}
	return req
}

// applyResult fills the model from a describe/write response. Credentials stay
// as configured: the API returns them masked.
func (m *APIToolProviderResourceModel) applyResult(result *APIToolProvider) {
	m.Name = types.StringValue(result.Name)
	m.Icon = &APIToolProviderIconModel{
		Content:    types.StringValue(result.Icon["content"]),
		Background: types.StringValue(result.Icon["background"]),
	}
	m.SchemaType = types.StringValue(result.SchemaType)
	m.Schema = types.StringValue(result.Schema)
	m.PrivacyPolicy = types.StringPointerValue(result.PrivacyPolicy)
	m.CustomDisclaimer = types.StringPointerValue(result.CustomDisclaimer)
	m.Labels = stringTypes(result.Labels)
	m.Tools = stringTypes(result.Tools)
}

// stringValues converts []types.String to []string.
func stringValues(values []types.String) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.ValueString())
	}
	return out
}

// stringTypes converts []string to []types.String.
func stringTypes(values []string) []types.String {
	out := make([]types.String, 0, len(values))
	for _, v := range values {
		out = append(out, types.StringValue(v))
	}
	return out
}

func (r *APIToolProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data APIToolProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateAPIToolProvider(ctx, data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create API tool provider: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIToolProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data APIToolProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.GetAPIToolProvider(ctx, data.Name.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read API tool provider: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIToolProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state APIToolProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PUT on the prior name; a differing request name renames the provider.
	result, err := r.client.UpdateAPIToolProvider(ctx, state.Name.ValueString(), data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update API tool provider: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *APIToolProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data APIToolProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAPIToolProvider(ctx, data.Name.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete API tool provider: %s", err))
		return
	}
}

func (r *APIToolProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
