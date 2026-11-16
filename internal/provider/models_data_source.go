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

var _ datasource.DataSource = &ModelsDataSource{}

func NewModelsDataSource() datasource.DataSource {
	return &ModelsDataSource{}
}

// ModelsDataSource defines the data source implementation.
type ModelsDataSource struct {
	client *DifyClient
}

// ModelItemModel describes a single model in the data source.
type ModelItemModel struct {
	Provider  types.String `tfsdk:"provider"`
	Model     types.String `tfsdk:"model"`
	ModelType types.String `tfsdk:"model_type"`
	Status    types.String `tfsdk:"status"`
}

// ModelsDataSourceModel describes the data source data model.
type ModelsDataSourceModel struct {
	Provider  types.String     `tfsdk:"provider_name"`
	ModelType types.String     `tfsdk:"model_type"`
	Models    []ModelItemModel `tfsdk:"models"`
}

func (d *ModelsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_models"
}

func (d *ModelsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists models available in the workspace across providers.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Optional provider filter (e.g. `langgenius/openai/openai`).",
				Optional:            true,
			},
			"model_type": schema.StringAttribute{
				MarkdownDescription: "Optional model type filter (e.g. `llm`, `text-embedding`, `rerank`, `speech2text`, `tts`).",
				Optional:            true,
			},
			"models": schema.ListNestedAttribute{
				MarkdownDescription: "List of models.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"provider": schema.StringAttribute{
							MarkdownDescription: "Provider id.",
							Computed:            true,
						},
						"model": schema.StringAttribute{
							MarkdownDescription: "Model name.",
							Computed:            true,
						},
						"model_type": schema.StringAttribute{
							MarkdownDescription: "Model type.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "`active` means usable now; anything else needs the provider set up.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ModelsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ModelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ModelsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	models, err := d.client.ListModels(ctx, data.Provider.ValueString(), data.ModelType.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list models: %s", err))
		return
	}

	items := make([]ModelItemModel, 0, len(models))
	for _, m := range models {
		items = append(items, ModelItemModel{
			Provider:  types.StringValue(m.Provider),
			Model:     types.StringValue(m.Model),
			ModelType: types.StringValue(m.ModelType),
			Status:    types.StringValue(m.Status),
		})
	}
	data.Models = items

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
