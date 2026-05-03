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

var _ datasource.DataSource = &AppDataSource{}

func NewAppDataSource() datasource.DataSource {
	return &AppDataSource{}
}

// AppDataSource defines the data source implementation.
type AppDataSource struct {
	client *DifyClient
}

// AppDataSourceModel describes the data source data model.
type AppDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Mode        types.String `tfsdk:"mode"`
	Description types.String `tfsdk:"description"`
	DSLYaml     types.String `tfsdk:"dsl_yaml"`
	EnableSite  types.Bool   `tfsdk:"enable_site"`
	EnableAPI   types.Bool   `tfsdk:"enable_api"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func (d *AppDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *AppDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Dify application including its DSL YAML export.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID to read.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "App name.",
				Computed:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "App mode.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "App description.",
				Computed:            true,
			},
			"dsl_yaml": schema.StringAttribute{
				MarkdownDescription: "DSL YAML export of the app.",
				Computed:            true,
			},
			"enable_site": schema.BoolAttribute{
				MarkdownDescription: "Whether the web app site is enabled.",
				Computed:            true,
			},
			"enable_api": schema.BoolAttribute{
				MarkdownDescription: "Whether the service API is enabled.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp.",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last update timestamp.",
				Computed:            true,
			},
		},
	}
}

func (d *AppDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AppDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := d.client.GetApp(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read app: %s", err))
		return
	}

	data.Name = types.StringValue(app.Name)
	data.Mode = types.StringValue(app.Mode)
	data.Description = types.StringValue(app.Description)
	data.DSLYaml = types.StringValue(app.DSLYaml)
	data.EnableSite = types.BoolValue(app.EnableSite)
	data.EnableAPI = types.BoolValue(app.EnableAPI)
	data.CreatedAt = types.StringValue(app.CreatedAt)
	data.UpdatedAt = types.StringValue(app.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
