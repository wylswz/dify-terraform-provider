// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatasetDocumentResource{}

func NewDatasetDocumentResource() resource.Resource {
	return &DatasetDocumentResource{}
}

// DatasetDocumentResource defines the resource implementation.
type DatasetDocumentResource struct {
	client *DifyClient
}

// DatasetDocumentResourceModel describes the resource data model.
type DatasetDocumentResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	DatasetID              types.String `tfsdk:"dataset_id"`
	Name                   types.String `tfsdk:"name"`
	DataSourceType         types.String `tfsdk:"data_source_type"`
	DataSourceInfo         types.Map    `tfsdk:"data_source_info"`
	IndexingStatus         types.String `tfsdk:"indexing_status"`
	IndexingTechnique      types.String `tfsdk:"indexing_technique"`
	EmbeddingModel         types.String `tfsdk:"embedding_model"`
	EmbeddingModelProvider types.String `tfsdk:"embedding_model_provider"`
}

func (r *DatasetDocumentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset_document"
}

func (r *DatasetDocumentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a document in a Dify dataset. " +
			"Documents are uploaded to datasets and asynchronously indexed/chunked.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Document ID (assigned by Dify after creation).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"dataset_id": schema.StringAttribute{
				MarkdownDescription: "Dataset ID to upload the document to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Document name.",
				Computed:            true,
			},
			"data_source_type": schema.StringAttribute{
				MarkdownDescription: "Data source type: `upload_file` or `text`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data_source_info": schema.MapAttribute{
				MarkdownDescription: "Data source content (e.g., `text_content` for text type, or file content for upload).",
				ElementType:         types.DynamicType,
				Required:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"indexing_status": schema.StringAttribute{
				MarkdownDescription: "Document indexing status (e.g., `indexing`, `completed`, `error`).",
				Computed:            true,
			},
			"indexing_technique": schema.StringAttribute{
				MarkdownDescription: "Indexing technique (defaults to dataset's technique if not specified).",
				Optional:            true,
			},
			"embedding_model": schema.StringAttribute{
				MarkdownDescription: "Embedding model name (required for high_quality indexing).",
				Optional:            true,
			},
			"embedding_model_provider": schema.StringAttribute{
				MarkdownDescription: "Embedding model provider (required for high_quality indexing).",
				Optional:            true,
			},
		},
	}
}

func (r *DatasetDocumentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatasetDocumentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatasetDocumentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert types.Map to map[string]any for data_source_info
	dataSourceInfoMap := make(map[string]any)
	for k, v := range data.DataSourceInfo.Elements() {
		dataSourceInfoMap[k] = v
	}

	createReq := DatasetDocumentCreateRequest{
		DataSourceType:         data.DataSourceType.ValueString(),
		DataSourceInfo:         dataSourceInfoMap,
		IndexingTechnique:      data.IndexingTechnique.ValueString(),
		EmbeddingModel:         data.EmbeddingModel.ValueString(),
		EmbeddingModelProvider: data.EmbeddingModelProvider.ValueString(),
	}

	document, err := r.client.CreateDatasetDocument(ctx, data.DatasetID.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create document: %s", err))
		return
	}

	data.ID = types.StringValue(document.ID)
	data.DatasetID = types.StringValue(document.DatasetID)
	data.Name = types.StringValue(document.Name)
	data.DataSourceType = types.StringValue(document.DataSourceType)
	data.IndexingStatus = types.StringValue(document.IndexingStatus)
	data.IndexingTechnique = types.StringValue(document.IndexingTechnique)
	data.EmbeddingModel = types.StringValue(document.EmbeddingModel)
	data.EmbeddingModelProvider = types.StringValue(document.EmbeddingModelProvider)

	// Set data_source_info from request
	data.DataSourceInfo, _ = types.MapValueFrom(ctx, types.DynamicType, dataSourceInfoMap)

	// Poll for indexing completion
	for i := 0; i < 300; i++ {
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Context Cancelled", "Document indexing polling cancelled")
			return
		case <-time.After(2 * time.Second):
		}

		doc, err := r.client.GetDatasetDocument(ctx, data.DatasetID.ValueString(), document.ID)
		if err != nil {
			continue
		}

		data.IndexingStatus = types.StringValue(doc.IndexingStatus)

		if doc.IndexingStatus == "completed" {
			break
		}
		if doc.IndexingStatus == "error" {
			resp.Diagnostics.AddError("Indexing Error", "Document indexing failed")
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetDocumentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatasetDocumentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	document, err := r.client.GetDatasetDocument(ctx, data.DatasetID.ValueString(), data.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read document: %s", err))
		return
	}

	data.ID = types.StringValue(document.ID)
	data.DatasetID = types.StringValue(document.DatasetID)
	data.Name = types.StringValue(document.Name)
	data.DataSourceType = types.StringValue(document.DataSourceType)
	data.IndexingStatus = types.StringValue(document.IndexingStatus)
	data.IndexingTechnique = types.StringValue(document.IndexingTechnique)
	data.EmbeddingModel = types.StringValue(document.EmbeddingModel)
	data.EmbeddingModelProvider = types.StringValue(document.EmbeddingModelProvider)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatasetDocumentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Documents cannot be updated, they must be recreated
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Documents cannot be updated. Delete and recreate the resource instead.",
	)
}

func (r *DatasetDocumentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatasetDocumentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDatasetDocument(ctx, data.DatasetID.ValueString(), data.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete document: %s", err))
		return
	}
}
