// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
	DataSourceInfo         types.String `tfsdk:"data_source_info"`
	IndexingStatus         types.String `tfsdk:"indexing_status"`
	IndexingTechnique      types.String `tfsdk:"indexing_technique"`
	EmbeddingModel         types.String `tfsdk:"embedding_model"`
	EmbeddingModelProvider types.String `tfsdk:"embedding_model_provider"`
	CreatorEmail           types.String `tfsdk:"creator_email"`
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
			"data_source_info": schema.StringAttribute{
				MarkdownDescription: "Data source content as a JSON string (e.g., `jsonencode({text_content = \"hello\"})` or `jsonencode({file_name = \"doc.pdf\", file_content = filebase64(\"doc.pdf\")})`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"indexing_status": schema.StringAttribute{
				MarkdownDescription: "Document indexing status (e.g., `indexing`, `completed`, `error`).",
				Computed:            true,
			},
			"indexing_technique": schema.StringAttribute{
				MarkdownDescription: "Indexing technique (defaults to dataset's technique if not specified).",
				Optional:            true,
				Computed:            true,
			},
			"embedding_model": schema.StringAttribute{
				MarkdownDescription: "Embedding model name (required for high_quality indexing). The server may normalize this.",
				Optional:            true,
				Computed:            true,
			},
			"embedding_model_provider": schema.StringAttribute{
				MarkdownDescription: "Embedding model provider (required for high_quality indexing). The server may normalize this.",
				Optional:            true,
				Computed:            true,
			},
			"creator_email": schema.StringAttribute{
				MarkdownDescription: "Email of the account that will own this document (must be an active account in the workspace).",
				Required:            true,
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

	// Parse data_source_info JSON string into map[string]any
	var dataSourceInfoMap map[string]any
	if err := json.Unmarshal([]byte(data.DataSourceInfo.ValueString()), &dataSourceInfoMap); err != nil {
		resp.Diagnostics.AddError("Invalid data_source_info", fmt.Sprintf("Unable to parse data_source_info JSON: %s", err))
		return
	}

	createReq := DatasetDocumentCreateRequest{
		DataSourceType:         data.DataSourceType.ValueString(),
		DataSourceInfo:         dataSourceInfoMap,
		IndexingTechnique:      data.IndexingTechnique.ValueString(),
		EmbeddingModel:         data.EmbeddingModel.ValueString(),
		EmbeddingModelProvider: data.EmbeddingModelProvider.ValueString(),
		CreatorEmail:           data.CreatorEmail.ValueString(),
	}

	document, err := r.client.CreateDatasetDocument(ctx, data.DatasetID.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create document: %s", err))
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("Created document: ID=%s, Name=%s, IndexingStatus=%q, DataSourceType=%s", document.ID, document.Name, document.IndexingStatus, document.DataSourceType))

	data.ID = types.StringValue(document.ID)
	// DocumentResponse does not include dataset_id; preserve from plan
	data.Name = types.StringValue(document.Name)
	data.IndexingStatus = types.StringValue(document.IndexingStatus)
	// data_source_type: backend converts "text" → "upload_file" internally; preserve user input
	// indexing_technique, embedding_model, embedding_model_provider: not in DocumentResponse; preserve from plan

	// Set data_source_info from request as JSON string
	dataSourceInfoJSON, _ := json.Marshal(dataSourceInfoMap)
	data.DataSourceInfo = types.StringValue(string(dataSourceInfoJSON))

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
			tflog.Warn(ctx, fmt.Sprintf("Poll %d: GetDatasetDocument error: %v", i, err))
			continue
		}

		data.IndexingStatus = types.StringValue(doc.IndexingStatus)
		tflog.Debug(ctx, fmt.Sprintf("Poll %d: indexing_status=%q, doc.ID=%s", i, doc.IndexingStatus, doc.ID))

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
	// DocumentResponse does not include dataset_id; preserve from state
	data.Name = types.StringValue(document.Name)
	data.IndexingStatus = types.StringValue(document.IndexingStatus)
	// data_source_type, indexing_technique, embedding_model, embedding_model_provider:
	// not reliably in DocumentResponse; preserve from state

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
