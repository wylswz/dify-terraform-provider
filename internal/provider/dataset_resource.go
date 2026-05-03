// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatasetResource{}

func NewDatasetResource() resource.Resource {
	return &DatasetResource{}
}

// DatasetResource defines the resource implementation.
type DatasetResource struct {
	client *DifyClient
}

// DatasetResourceModel describes the resource data model.
type DatasetResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	IndexingTechnique      types.String `tfsdk:"indexing_technique"`
	Permission             types.String `tfsdk:"permission"`
	ProcessRule            types.String `tfsdk:"process_rule"`
	EmbeddingModel         types.String `tfsdk:"embedding_model"`
	EmbeddingModelProvider types.String `tfsdk:"embedding_model_provider"`
	CreatorEmail           types.String `tfsdk:"creator_email"`
}

func (r *DatasetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset"
}

func (r *DatasetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Dify dataset. " +
			"Datasets are collections of documents that can be used for knowledge retrieval in applications.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Dataset ID (assigned by Dify after creation).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Dataset name (1-40 characters).",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Dataset description (max 400 characters).",
				Optional:            true,
			},
			"indexing_technique": schema.StringAttribute{
				MarkdownDescription: "Indexing technique: `high_quality` (requires embedding model) or `economy`.",
				Optional:            true,
			},
			"permission": schema.StringAttribute{
				MarkdownDescription: "Dataset permission: `only_me`, `all_team_members`, or `partial_members`.",
				Optional:            true,
			},
			"process_rule": schema.StringAttribute{
				MarkdownDescription: "Chunking strategy configuration as a JSON string (e.g., `jsonencode({mode = \"automatic\", rules = {chunk_size = 500}})`).",
				Optional:            true,
			},
			"embedding_model": schema.StringAttribute{
				MarkdownDescription: "Embedding model name (required for high_quality indexing). The server may normalize this to the default model.",
				Optional:            true,
				Computed:            true,
			},
			"embedding_model_provider": schema.StringAttribute{
				MarkdownDescription: "Embedding model provider (required for high_quality indexing). The server may normalize this value.",
				Optional:            true,
				Computed:            true,
			},
			"creator_email": schema.StringAttribute{
				MarkdownDescription: "Email of the account that will own this dataset (must be an active account in the workspace).",
				Required:            true,
			},
		},
	}
}

func (r *DatasetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatasetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse process_rule JSON string into map[string]any
	var processRuleMap map[string]any
	if !data.ProcessRule.IsNull() && !data.ProcessRule.IsUnknown() {
		if err := json.Unmarshal([]byte(data.ProcessRule.ValueString()), &processRuleMap); err != nil {
			resp.Diagnostics.AddError("Invalid process_rule", fmt.Sprintf("Unable to parse process_rule JSON: %s", err))
			return
		}
	}

	createReq := DatasetCreateRequest{
		Name:                   data.Name.ValueString(),
		Description:            data.Description.ValueString(),
		IndexingTechnique:      data.IndexingTechnique.ValueString(),
		Permission:             data.Permission.ValueString(),
		ProcessRule:            processRuleMap,
		EmbeddingModel:         data.EmbeddingModel.ValueString(),
		EmbeddingModelProvider: data.EmbeddingModelProvider.ValueString(),
		CreatorEmail:           data.CreatorEmail.ValueString(),
	}

	dataset, err := r.client.CreateDataset(ctx, createReq)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 409 {
			resp.Diagnostics.AddError("Conflict", "Dataset name already exists")
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create dataset: %s", err))
		return
	}

	data.ID = types.StringValue(dataset.ID)
	data.Name = types.StringValue(dataset.Name)
	data.Description = types.StringValue(dataset.Description)
	data.IndexingTechnique = types.StringValue(dataset.IndexingTechnique)
	data.Permission = types.StringValue(dataset.Permission)
	data.EmbeddingModel = types.StringValue(dataset.EmbeddingModel)
	data.EmbeddingModelProvider = types.StringValue(dataset.EmbeddingModelProvider)

	// Set process_rule from response as JSON string
	if dataset.ProcessRule != nil {
		processRuleJSON, _ := json.Marshal(dataset.ProcessRule)
		data.ProcessRule = types.StringValue(string(processRuleJSON))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dataset, err := r.client.GetDataset(ctx, data.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read dataset: %s", err))
		return
	}

	data.ID = types.StringValue(dataset.ID)
	data.Name = types.StringValue(dataset.Name)
	data.Description = types.StringValue(dataset.Description)
	data.IndexingTechnique = types.StringValue(dataset.IndexingTechnique)
	data.Permission = types.StringValue(dataset.Permission)
	data.EmbeddingModel = types.StringValue(dataset.EmbeddingModel)
	data.EmbeddingModelProvider = types.StringValue(dataset.EmbeddingModelProvider)

	// Set process_rule from response as JSON string
	if dataset.ProcessRule != nil {
		processRuleJSON, _ := json.Marshal(dataset.ProcessRule)
		data.ProcessRule = types.StringValue(string(processRuleJSON))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse process_rule JSON string into map[string]any
	var processRuleMap map[string]any
	if !data.ProcessRule.IsNull() && !data.ProcessRule.IsUnknown() {
		if err := json.Unmarshal([]byte(data.ProcessRule.ValueString()), &processRuleMap); err != nil {
			resp.Diagnostics.AddError("Invalid process_rule", fmt.Sprintf("Unable to parse process_rule JSON: %s", err))
			return
		}
	}

	updateReq := DatasetUpdateRequest{
		Name:                   data.Name.ValueString(),
		Description:            data.Description.ValueString(),
		IndexingTechnique:      data.IndexingTechnique.ValueString(),
		Permission:             data.Permission.ValueString(),
		ProcessRule:            processRuleMap,
		EmbeddingModel:         data.EmbeddingModel.ValueString(),
		EmbeddingModelProvider: data.EmbeddingModelProvider.ValueString(),
	}

	dataset, err := r.client.UpdateDataset(ctx, data.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update dataset: %s", err))
		return
	}

	data.ID = types.StringValue(dataset.ID)
	data.Name = types.StringValue(dataset.Name)
	data.Description = types.StringValue(dataset.Description)
	data.IndexingTechnique = types.StringValue(dataset.IndexingTechnique)
	data.Permission = types.StringValue(dataset.Permission)
	data.EmbeddingModel = types.StringValue(dataset.EmbeddingModel)
	data.EmbeddingModelProvider = types.StringValue(dataset.EmbeddingModelProvider)

	// Set process_rule from response as JSON string
	if dataset.ProcessRule != nil {
		processRuleJSON, _ := json.Marshal(dataset.ProcessRule)
		data.ProcessRule = types.StringValue(string(processRuleJSON))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDataset(ctx, data.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete dataset: %s", err))
		return
	}
}
