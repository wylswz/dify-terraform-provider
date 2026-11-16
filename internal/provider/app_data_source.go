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
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func (d *AppDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *AppDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Dify application by `id` or exact `name` (exactly one required).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID to read. Exactly one of `id` or `name` is required.",
				Optional:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Exact app name to look up. Exactly one of `id` or `name` is required.",
				Optional:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "App mode.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "App description.",
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

	appID := data.ID.ValueString()
	appName := data.Name.ValueString()

	if (appID == "") == (appName == "") {
		resp.Diagnostics.AddError(
			"Invalid App Lookup",
			"Exactly one of `id` or `name` must be provided.",
		)
		return
	}

	// The apps list `name` filter also resolves an app id, so both lookups go
	// through the same endpoint; results are then matched exactly.
	lookup := appName
	if appID != "" {
		lookup = appID
	}

	apps, err := d.client.ListApps(ctx, lookup)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list apps: %s", err))
		return
	}

	var matches []AppListItem
	for _, app := range apps {
		if appID != "" {
			if app.ID == appID {
				matches = append(matches, app)
			}
		} else if app.Name == appName {
			matches = append(matches, app)
		}
	}

	switch len(matches) {
	case 0:
		if appID != "" {
			resp.Diagnostics.AddError("App Not Found", fmt.Sprintf("No app found with id '%s'", appID))
		} else {
			resp.Diagnostics.AddError("App Not Found", fmt.Sprintf("No app found named '%s'", appName))
		}
		return
	case 1:
	default:
		resp.Diagnostics.AddError("Ambiguous App Name", fmt.Sprintf("%d apps are named '%s'; use `id` instead", len(matches), appName))
		return
	}

	app := matches[0]
	data.ID = types.StringValue(app.ID)
	data.Name = types.StringValue(app.Name)
	data.Mode = types.StringValue(app.Mode)
	data.Description = types.StringValue(app.Description)
	data.UpdatedAt = types.StringValue(app.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
