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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatasetResource{}
var _ resource.ResourceWithImportState = &DatasetResource{}

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
	Provider               types.String `tfsdk:"provider_type"`
	EmbeddingModel         types.String `tfsdk:"embedding_model"`
	EmbeddingModelProvider types.String `tfsdk:"embedding_model_provider"`
	RetrievalModel         types.String `tfsdk:"retrieval_model"`
	ExternalKnowledgeAPIId types.String `tfsdk:"external_knowledge_api_id"`
	ExternalKnowledgeId    types.String `tfsdk:"external_knowledge_id"`
	CreatedAt              types.Int64  `tfsdk:"created_at"`
	UpdatedAt              types.Int64  `tfsdk:"updated_at"`
}

func (r *DatasetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset"
}

func (r *DatasetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Dify knowledge base (dataset). " +
			"The dataset stores documents and supports retrieval-augmented generation. " +
			"`retrieval_model` is a JSON string passed through to the API. " +
			"Import by id: `terraform import dify_dataset.x <id>`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Dataset ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the knowledge base (1-40 characters).",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the knowledge base.",
				Optional:            true,
			},
			"indexing_technique": schema.StringAttribute{
				MarkdownDescription: "`high_quality` uses embedding models for precise search; `economy` uses keyword-based indexing.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("high_quality"),
			},
			"permission": schema.StringAttribute{
				MarkdownDescription: "`only_me` restricts access to the creator, `all_team_members` grants workspace-wide access, `partial_members` grants access to specified members.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("only_me"),
			},
			"provider_type": schema.StringAttribute{
				MarkdownDescription: "`vendor` for internal knowledge bases, `external` for external ones.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("vendor"),
			},
			"embedding_model": schema.StringAttribute{
				MarkdownDescription: "Embedding model name. Use the `model` field from Get Available Models with `model_type=text-embedding`.",
				Optional:            true,
			},
			"embedding_model_provider": schema.StringAttribute{
				MarkdownDescription: "Embedding model provider. Use the `provider` field from Get Available Models with `model_type=text-embedding`.",
				Optional:            true,
			},
			"retrieval_model": schema.StringAttribute{
				MarkdownDescription: "Retrieval model configuration as a JSON string. Controls how chunks are searched and ranked.",
				Optional:            true,
				Computed:            true,
			},
			"external_knowledge_api_id": schema.StringAttribute{
				MarkdownDescription: "ID of the external knowledge API (when `provider` is `external`).",
				Optional:            true,
			},
			"external_knowledge_id": schema.StringAttribute{
				MarkdownDescription: "ID of the external knowledge base (when `provider` is `external`).",
				Optional:            true,
			},
			"created_at": schema.Int64Attribute{
				MarkdownDescription: "Unix seconds when the dataset was created.",
				Computed:            true,
			},
			"updated_at": schema.Int64Attribute{
				MarkdownDescription: "Unix seconds when the dataset was last updated.",
				Computed:            true,
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

// request converts the model to the API payload.
func (m *DatasetResourceModel) request() DatasetCreateRequest {
	req := DatasetCreateRequest{
		Name: m.Name.ValueString(),
	}
	if known(m.Description) {
		req.Description = m.Description.ValueString()
	}
	if known(m.IndexingTechnique) {
		req.IndexingTechnique = stringPointer(m.IndexingTechnique.ValueString())
	}
	if known(m.Permission) {
		req.Permission = stringPointer(m.Permission.ValueString())
	}
	if known(m.Provider) {
		req.Provider = stringPointer(m.Provider.ValueString())
	}
	if known(m.EmbeddingModel) {
		req.EmbeddingModel = stringPointer(m.EmbeddingModel.ValueString())
	}
	if known(m.EmbeddingModelProvider) {
		req.EmbeddingModelProvider = stringPointer(m.EmbeddingModelProvider.ValueString())
	}
	if known(m.RetrievalModel) {
		raw := json.RawMessage(m.RetrievalModel.ValueString())
		req.RetrievalModel = &raw
	}
	if known(m.ExternalKnowledgeAPIId) {
		req.ExternalKnowledgeAPIId = stringPointer(m.ExternalKnowledgeAPIId.ValueString())
	}
	if known(m.ExternalKnowledgeId) {
		req.ExternalKnowledgeId = stringPointer(m.ExternalKnowledgeId.ValueString())
	}
	return req
}

// updateRequest converts the model to the PATCH payload.
func (m *DatasetResourceModel) updateRequest() DatasetUpdateRequest {
	req := DatasetUpdateRequest{}
	if !m.Name.IsNull() {
		req.Name = stringPointer(m.Name.ValueString())
	}
	if known(m.Description) {
		req.Description = stringPointer(m.Description.ValueString())
	}
	if known(m.IndexingTechnique) {
		req.IndexingTechnique = stringPointer(m.IndexingTechnique.ValueString())
	}
	if known(m.Permission) {
		req.Permission = stringPointer(m.Permission.ValueString())
	}
	if known(m.Provider) {
		req.Provider = stringPointer(m.Provider.ValueString())
	}
	if known(m.EmbeddingModel) {
		req.EmbeddingModel = stringPointer(m.EmbeddingModel.ValueString())
	}
	if known(m.EmbeddingModelProvider) {
		req.EmbeddingModelProvider = stringPointer(m.EmbeddingModelProvider.ValueString())
	}
	if known(m.RetrievalModel) {
		raw := json.RawMessage(m.RetrievalModel.ValueString())
		req.RetrievalModel = &raw
	}
	if known(m.ExternalKnowledgeAPIId) {
		req.ExternalKnowledgeAPIId = stringPointer(m.ExternalKnowledgeAPIId.ValueString())
	}
	if known(m.ExternalKnowledgeId) {
		req.ExternalKnowledgeId = stringPointer(m.ExternalKnowledgeId.ValueString())
	}
	return req
}

// stringPointer returns a pointer to s.
func stringPointer(s string) *string {
	return &s
}

// known reports whether a planned string attribute carries a concrete value;
// computed attributes are unknown during create and must not be sent.
func known(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// nullIfEmpty maps an empty API string to null so optional attributes that
// were not configured stay null in state instead of drifting to "".
func nullIfEmpty(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// applyResult fills the model from a describe/write response.
func (m *DatasetResourceModel) applyResult(result *Dataset) {
	m.ID = types.StringValue(result.ID)
	m.Name = types.StringValue(result.Name)
	m.Description = nullIfEmpty(result.Description)
	m.IndexingTechnique = types.StringValue(result.IndexingTechnique)
	m.Permission = types.StringValue(result.Permission)
	m.Provider = types.StringValue(result.Provider)
	m.EmbeddingModel = nullIfEmpty(result.EmbeddingModel)
	m.EmbeddingModelProvider = nullIfEmpty(result.EmbeddingModelProvider)
	if len(result.RetrievalModel) > 0 {
		m.RetrievalModel = types.StringValue(string(result.RetrievalModel))
	} else {
		m.RetrievalModel = types.StringNull()
	}
	m.ExternalKnowledgeAPIId = nullIfEmpty(result.ExternalKnowledgeAPIId)
	m.ExternalKnowledgeId = nullIfEmpty(result.ExternalKnowledgeId)
	m.CreatedAt = types.Int64Value(result.CreatedAt)
	m.UpdatedAt = types.Int64Value(result.UpdatedAt)
}

func (r *DatasetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateDataset(ctx, data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create dataset: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.GetDataset(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read dataset: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.UpdateDataset(ctx, data.ID.ValueString(), data.updateRequest())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update dataset: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatasetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDataset(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete dataset: %s", err))
		return
	}
}

func (r *DatasetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

