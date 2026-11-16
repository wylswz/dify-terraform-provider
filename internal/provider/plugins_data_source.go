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

var _ datasource.DataSource = &PluginsDataSource{}

func NewPluginsDataSource() datasource.DataSource {
	return &PluginsDataSource{}
}

// PluginsDataSource defines the data source implementation.
type PluginsDataSource struct {
	client *DifyClient
}

// PluginItemModel describes a single installed plugin in the data source.
type PluginItemModel struct {
	PluginID   types.String `tfsdk:"plugin_id"`
	Identifier types.String `tfsdk:"identifier"`
	Version    types.String `tfsdk:"version"`
	Category   types.String `tfsdk:"category"`
	Source     types.String `tfsdk:"source"`
}

// PluginsDataSourceModel describes the data source data model.
type PluginsDataSourceModel struct {
	Plugins []PluginItemModel `tfsdk:"plugins"`
}

func (d *PluginsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugins"
}

func (d *PluginsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all plugins installed in the workspace.",

		Attributes: map[string]schema.Attribute{
			"plugins": schema.ListNestedAttribute{
				MarkdownDescription: "List of installed plugins.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"plugin_id": schema.StringAttribute{
							MarkdownDescription: "Plugin ID (e.g. `langgenius/openai`).",
							Computed:            true,
						},
						"identifier": schema.StringAttribute{
							MarkdownDescription: "Versioned plugin identifier (e.g. `langgenius/openai:0.3.8@<sha256>`).",
							Computed:            true,
						},
						"version": schema.StringAttribute{
							MarkdownDescription: "Installed version.",
							Computed:            true,
						},
						"category": schema.StringAttribute{
							MarkdownDescription: "Plugin category (e.g. `model`, `tool`).",
							Computed:            true,
						},
						"source": schema.StringAttribute{
							MarkdownDescription: "Installation source.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *PluginsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PluginsDataSourceModel

	plugins, err := d.client.ListPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list plugins: %s", err))
		return
	}

	items := make([]PluginItemModel, 0, len(plugins))
	for _, p := range plugins {
		items = append(items, PluginItemModel{
			PluginID:   types.StringValue(p.PluginID),
			Identifier: types.StringValue(p.Identifier),
			Version:    types.StringValue(p.Version),
			Category:   types.StringValue(p.Category),
			Source:     types.StringValue(p.Source),
		})
	}
	data.Plugins = items

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
