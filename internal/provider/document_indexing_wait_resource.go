// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DocumentIndexingWaitResource{}
var _ resource.ResourceWithImportState = &DocumentIndexingWaitResource{}

func NewDocumentIndexingWaitResource() resource.Resource {
	return &DocumentIndexingWaitResource{}
}

// DocumentIndexingWaitResource is a standalone resource that only waits for a
// document's indexing to reach a terminal state. It does not create or delete
// the document itself. Use it when `dify_dataset_document.indexing_wait` is
// disabled and you want explicit control over the wait timeout/interval.
type DocumentIndexingWaitResource struct {
	client *DifyClient
}

// DocumentIndexingWaitResourceModel describes the resource data model.
type DocumentIndexingWaitResourceModel struct {
	DatasetID       types.String `tfsdk:"dataset_id"`
	DocumentID      types.String `tfsdk:"document_id"`
	Timeout         types.Int64  `tfsdk:"timeout"`
	PollingInterval types.Int64  `tfsdk:"polling_interval"`
	IndexingStatus  types.String `tfsdk:"indexing_status"`
	IndexingError   types.String `tfsdk:"indexing_error"`
	CompletedAt     types.Int64  `tfsdk:"completed_at"`
}

func (r *DocumentIndexingWaitResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset_document_indexing_wait"
}

func (r *DocumentIndexingWaitResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Waits for a Dify dataset document's indexing to reach `available` or `failed`. " +
			"This resource does not create or delete the document; it only blocks until indexing completes. " +
			"Pair it with `dify_dataset_document` where `indexing_wait = false`. " +
			"Import by `dataset_id:document_id`.",

		Attributes: map[string]schema.Attribute{
			"dataset_id": schema.StringAttribute{
				MarkdownDescription: "The knowledge base the document belongs to.",
				Required:            true,
			},
			"document_id": schema.StringAttribute{
				MarkdownDescription: "The document to wait for.",
				Required:            true,
			},
			"timeout": schema.Int64Attribute{
				MarkdownDescription: "Maximum seconds to wait for indexing to finish.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"polling_interval": schema.Int64Attribute{
				MarkdownDescription: "Seconds between indexing status polls.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3),
			},
			"indexing_status": schema.StringAttribute{
				MarkdownDescription: "Final indexing status (`available` or `failed`).",
				Computed:            true,
			},
			"indexing_error": schema.StringAttribute{
				MarkdownDescription: "Indexing error message when status is `failed`.",
				Computed:            true,
			},
			"completed_at": schema.Int64Attribute{
				MarkdownDescription: "Unix seconds when indexing completed.",
				Computed:            true,
			},
		},
	}
}

func (r *DocumentIndexingWaitResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DocumentIndexingWaitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DocumentIndexingWaitResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout := time.Duration(data.Timeout.ValueInt64()) * time.Second
	interval := time.Duration(data.PollingInterval.ValueInt64()) * time.Second

	doc, err := r.client.WaitForDocumentIndexing(ctx, data.DatasetID.ValueString(), data.DocumentID.ValueString(), timeout, interval)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Document indexing did not complete: %s", err))
		return
	}

	data.IndexingStatus = types.StringValue(indexingStatus(doc.DisplayStatus))
	data.IndexingError = types.StringValue(doc.Error)
	if doc.CompletedAt != nil {
		data.CompletedAt = types.Int64Value(*doc.CompletedAt)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentIndexingWaitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DocumentIndexingWaitResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read does not block: it only refreshes the cached status from the API.
	doc, err := r.client.GetDocument(ctx, data.DatasetID.ValueString(), data.DocumentID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read document: %s", err))
		return
	}

	data.IndexingStatus = types.StringValue(indexingStatus(doc.DisplayStatus))
	data.IndexingError = types.StringValue(doc.Error)
	if doc.CompletedAt != nil {
		data.CompletedAt = types.Int64Value(*doc.CompletedAt)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentIndexingWaitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DocumentIndexingWaitResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the dataset or document changed, re-wait for the new document. Timeout
	// and interval changes alone do not trigger a re-wait.
	timeout := time.Duration(data.Timeout.ValueInt64()) * time.Second
	interval := time.Duration(data.PollingInterval.ValueInt64()) * time.Second

	doc, err := r.client.WaitForDocumentIndexing(ctx, data.DatasetID.ValueString(), data.DocumentID.ValueString(), timeout, interval)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Document indexing did not complete: %s", err))
		return
	}

	data.IndexingStatus = types.StringValue(indexingStatus(doc.DisplayStatus))
	data.IndexingError = types.StringValue(doc.Error)
	if doc.CompletedAt != nil {
		data.CompletedAt = types.Int64Value(*doc.CompletedAt)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentIndexingWaitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No remote resource to delete; just remove from state.
}

func (r *DocumentIndexingWaitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	datasetID, documentID, err := splitImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dataset_id"), datasetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("document_id"), documentID)...)
}
