// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ToolsDataSource{}

func NewToolsDataSource() datasource.DataSource {
	return &ToolsDataSource{}
}

// ToolsDataSource defines the data source implementation.
type ToolsDataSource struct {
	client *DifyClient
}

// ToolItemModel describes a single tool in the data source.
type ToolItemModel struct {
	Provider     types.String `tfsdk:"provider"`
	ProviderType types.String `tfsdk:"provider_type"`
	Name         types.String `tfsdk:"name"`
	Label        types.String `tfsdk:"label"`
	NodeData     types.String `tfsdk:"node_data"`
}

// ToolsDataSourceModel describes the data source data model.
type ToolsDataSourceModel struct {
	Provider types.String    `tfsdk:"provider_name"`
	Query    types.String    `tfsdk:"query"`
	Tools    []ToolItemModel `tfsdk:"tools"`
}

func (d *ToolsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tools"
}

func (d *ToolsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists tools available in the workspace. `node_data` carries the " +
			"server-provided workflow tool-node template as JSON — the exact `provider_id`, " +
			"`provider_name`, and `plugin_unique_identifier` a tool node's DSL expects.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Optional provider filter (e.g. `langgenius/tavily/tavily`).",
				Optional:            true,
			},
			"query": schema.StringAttribute{
				MarkdownDescription: "Optional words to match in the tool name or label, or the provider id or label.",
				Optional:            true,
			},
			"tools": schema.ListNestedAttribute{
				MarkdownDescription: "List of tools.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"provider": schema.StringAttribute{
							MarkdownDescription: "Provider id.",
							Computed:            true,
						},
						"provider_type": schema.StringAttribute{
							MarkdownDescription: "Provider type (`builtin`, `api`, `mcp`, `workflow`).",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Tool name.",
							Computed:            true,
						},
						"label": schema.StringAttribute{
							MarkdownDescription: "Tool label.",
							Computed:            true,
						},
						"node_data": schema.StringAttribute{
							MarkdownDescription: "Workflow tool-node template for this tool, as JSON.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ToolsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*DifyClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *DifyClient, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *ToolsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ToolsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tools, err := d.client.ListTools(ctx, data.Provider.ValueString(), data.Query.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list tools: %s", err))
		return
	}

	items := make([]ToolItemModel, 0, len(tools))
	for _, t := range tools {
		items = append(items, ToolItemModel{
			Provider:     types.StringValue(t.Provider),
			ProviderType: types.StringValue(t.ProviderType),
			Name:         types.StringValue(t.Name),
			Label:        types.StringValue(t.Label),
			NodeData:     types.StringValue(string(t.NodeData)),
		})
	}
	data.Tools = items

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
