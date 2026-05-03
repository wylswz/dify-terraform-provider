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
	PluginUniqueIdentifier types.String `tfsdk:"plugin_unique_identifier"`
	PluginInstallationID   types.String `tfsdk:"plugin_installation_id"`
	Name                   types.String `tfsdk:"name"`
}

// PluginsDataSourceModel describes the data source data model.
type PluginsDataSourceModel struct {
	Plugins []PluginItemModel `tfsdk:"plugins"`
	Total   types.Int64       `tfsdk:"total"`
}

func (d *PluginsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugins"
}

func (d *PluginsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all installed plugins in the workspace.",

		Attributes: map[string]schema.Attribute{
			"plugins": schema.ListNestedAttribute{
				MarkdownDescription: "List of installed plugins.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"plugin_unique_identifier": schema.StringAttribute{
							MarkdownDescription: "Unique identifier for the plugin.",
							Computed:            true,
						},
						"plugin_installation_id": schema.StringAttribute{
							MarkdownDescription: "Installation ID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Plugin name.",
							Computed:            true,
						},
					},
				},
			},
			"total": schema.Int64Attribute{
				MarkdownDescription: "Total number of installed plugins.",
				Computed:            true,
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

	result, err := d.client.ListPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list plugins: %s", err))
		return
	}

	plugins := make([]PluginItemModel, 0, len(result.Plugins))
	for _, p := range result.Plugins {
		plugins = append(plugins, PluginItemModel{
			PluginUniqueIdentifier: types.StringValue(p.PluginUniqueIdentifier),
			PluginInstallationID:   types.StringValue(p.PluginInstallationID),
			Name:                   types.StringValue(p.Name),
		})
	}
	data.Plugins = plugins
	data.Total = types.Int64Value(int64(result.Total))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
