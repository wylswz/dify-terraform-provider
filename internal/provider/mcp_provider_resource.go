// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &MCPProviderResource{}
var _ resource.ResourceWithImportState = &MCPProviderResource{}

func NewMCPProviderResource() resource.Resource {
	return &MCPProviderResource{}
}

// MCPProviderResource defines the resource implementation.
type MCPProviderResource struct {
	client *DifyClient
}

// MCPProviderResourceModel describes the resource data model.
type MCPProviderResourceModel struct {
	ID               types.String   `tfsdk:"id"`
	Name             types.String   `tfsdk:"name"`
	ServerURL        types.String   `tfsdk:"server_url"`
	ServerIdentifier types.String   `tfsdk:"server_identifier"`
	Icon             types.String   `tfsdk:"icon"`
	IconType         types.String   `tfsdk:"icon_type"`
	IconBackground   types.String   `tfsdk:"icon_background"`
	Headers          types.Map      `tfsdk:"headers"`
	Timeout          types.Float64  `tfsdk:"timeout"`
	SSEReadTimeout   types.Float64  `tfsdk:"sse_read_timeout"`
	Tools            []types.String `tfsdk:"tools"`
}

func (r *MCPProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_provider"
}

func (r *MCPProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an MCP provider — a remote MCP server the workspace calls " +
			"tools from. `server_url` and `headers` are write-only: describe returns them masked, " +
			"so state keeps the configured values. Import by id: " +
			"`terraform import dify_mcp_provider.x <id>`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Provider ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Provider name.",
				Required:            true,
			},
			"server_url": schema.StringAttribute{
				MarkdownDescription: "MCP server URL. Write-only and sensitive; the API returns it masked.",
				Required:            true,
				Sensitive:           true,
			},
			"server_identifier": schema.StringAttribute{
				MarkdownDescription: "Unique identifier of the server within the workspace.",
				Required:            true,
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Provider icon (emoji character when `icon_type` is `emoji`).",
				Required:            true,
			},
			"icon_type": schema.StringAttribute{
				MarkdownDescription: "Icon type (`emoji`, `link`, `image`).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("emoji"),
			},
			"icon_background": schema.StringAttribute{
				MarkdownDescription: "Icon background color.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"headers": schema.MapAttribute{
				MarkdownDescription: "Extra HTTP headers sent to the server. Write-only and sensitive.",
				ElementType:         types.StringType,
				Optional:            true,
				Sensitive:           true,
			},
			"timeout": schema.Float64Attribute{
				MarkdownDescription: "Connection timeout in seconds (sent as `configuration.timeout`).",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(30),
			},
			"sse_read_timeout": schema.Float64Attribute{
				MarkdownDescription: "SSE read timeout in seconds (sent as `configuration.sse_read_timeout`).",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(300),
			},
			"tools": schema.ListAttribute{
				MarkdownDescription: "Names of the tools the server advertised.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (r *MCPProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m *MCPProviderResourceModel) request() mcpProviderRequest {
	return mcpProviderRequest{
		ServerURL:        m.ServerURL.ValueString(),
		Name:             m.Name.ValueString(),
		Icon:             m.Icon.ValueString(),
		IconType:         m.IconType.ValueString(),
		IconBackground:   m.IconBackground.ValueString(),
		ServerIdentifier: m.ServerIdentifier.ValueString(),
		Configuration: &MCPConfiguration{
			Timeout:        m.Timeout.ValueFloat64(),
			SSEReadTimeout: m.SSEReadTimeout.ValueFloat64(),
		},
		Headers: stringMap(m.Headers),
	}
}

// stringMap converts a string-element types.Map to map[string]string; a null or
// unknown map yields nil.
func stringMap(m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	for k, v := range m.Elements() {
		if s, ok := v.(types.String); ok {
			out[k] = s.ValueString()
		}
	}
	return out
}

// applyResult fills the model from a describe/write response. server_url and
// headers stay as configured: the API returns them masked.
func (m *MCPProviderResourceModel) applyResult(result *MCPProvider) {
	m.ID = types.StringValue(result.ID)
	m.Name = types.StringValue(result.Name)

	var icon string
	if err := json.Unmarshal(result.Icon, &icon); err == nil {
		m.Icon = types.StringValue(icon)
	}
	m.ServerIdentifier = types.StringPointerValue(result.ServerIdentifier)
	if result.Configuration != nil {
		m.Timeout = types.Float64Value(result.Configuration.Timeout)
		m.SSEReadTimeout = types.Float64Value(result.Configuration.SSEReadTimeout)
	}
	m.Tools = stringTypes(result.Tools)
}

// syncTools records the provider's tools, refreshing once when the write came
// back without any (the server fetches tools best-effort on create/update).
func (r *MCPProviderResource) syncTools(ctx context.Context, data *MCPProviderResourceModel) error {
	if len(data.Tools) == 0 {
		result, err := r.client.RefreshMCPProvider(ctx, data.ID.ValueString())
		if err != nil {
			return fmt.Errorf("unable to refresh MCP provider tools: %s", err)
		}
		data.Tools = stringTypes(result.Tools)
	}
	return nil
}

func (r *MCPProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MCPProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateMCPProvider(ctx, data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create MCP provider: %s", err))
		return
	}

	data.applyResult(result)
	if err := r.syncTools(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MCPProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data MCPProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.GetMCPProvider(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read MCP provider: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MCPProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data MCPProviderResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.UpdateMCPProvider(ctx, data.ID.ValueString(), data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update MCP provider: %s", err))
		return
	}

	data.applyResult(result)
	if err := r.syncTools(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *MCPProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data MCPProviderResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteMCPProvider(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete MCP provider: %s", err))
		return
	}
}

func (r *MCPProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
