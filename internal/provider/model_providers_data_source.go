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

var _ datasource.DataSource = &ModelProvidersDataSource{}

func NewModelProvidersDataSource() datasource.DataSource {
	return &ModelProvidersDataSource{}
}

// ModelProvidersDataSource defines the data source implementation.
type ModelProvidersDataSource struct {
	client *DifyClient
}

// ModelProviderItemModel describes a single model provider in the data source.
type ModelProviderItemModel struct {
	Provider types.String `tfsdk:"provider"`
}

// ModelProvidersDataSourceModel describes the data source data model.
type ModelProvidersDataSourceModel struct {
	Providers []ModelProviderItemModel `tfsdk:"providers"`
}

func (d *ModelProvidersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_model_providers"
}

func (d *ModelProvidersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all model providers configured in the workspace.",

		Attributes: map[string]schema.Attribute{
			"providers": schema.ListNestedAttribute{
				MarkdownDescription: "List of model providers.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"provider": schema.StringAttribute{
							MarkdownDescription: "Provider name.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ModelProvidersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ModelProvidersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ModelProvidersDataSourceModel

	result, err := d.client.ListModelProviders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list model providers: %s", err))
		return
	}

	providers := make([]ModelProviderItemModel, 0, len(result.Data))
	for _, p := range result.Data {
		providers = append(providers, ModelProviderItemModel{
			Provider: types.StringValue(p.Provider),
		})
	}
	data.Providers = providers

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
