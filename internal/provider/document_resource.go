// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DocumentResource{}
var _ resource.ResourceWithImportState = &DocumentResource{}

func NewDocumentResource() resource.Resource {
	return &DocumentResource{}
}

// DocumentResource defines the resource implementation.
type DocumentResource struct {
	client *DifyClient
}

// DocumentResourceModel describes the resource data model.
type DocumentResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	DatasetID              types.String `tfsdk:"dataset_id"`
	Name                   types.String `tfsdk:"name"`
	Text                   types.String `tfsdk:"text"`
	ProcessRule            types.String `tfsdk:"process_rule"`
	DocForm                types.String `tfsdk:"doc_form"`
	DocLanguage            types.String `tfsdk:"doc_language"`
	IndexingTechnique      types.String `tfsdk:"indexing_technique"`
	RetrievalModel         types.String `tfsdk:"retrieval_model"`
	EmbeddingModel         types.String `tfsdk:"embedding_model"`
	EmbeddingModelProvider types.String `tfsdk:"embedding_model_provider"`
	// indexing_wait controls whether Create/Update block until the document's
	// indexing reaches "available". When false, the resource returns immediately
	// and the companion dify_dataset_document_indexing_wait resource can be used
	// to wait explicitly.
	IndexingWait         types.Bool   `tfsdk:"indexing_wait"`
	IndexingTimeout      types.Int64  `tfsdk:"indexing_timeout"`
	IndexingPollInterval types.Int64  `tfsdk:"indexing_poll_interval"`
	IndexingStatus       types.String `tfsdk:"indexing_status"`
	IndexingError        types.String `tfsdk:"indexing_error"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	Archived             types.Bool   `tfsdk:"archived"`
	CreatedAt            types.Int64  `tfsdk:"created_at"`
	UpdatedAt            types.Int64  `tfsdk:"updated_at"`
}

func (r *DocumentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset_document"
}

func (r *DocumentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a document inside a Dify knowledge base (dataset). " +
			"The document is created from text and indexed asynchronously. " +
			"Set `indexing_wait = false` to skip blocking on indexing and use " +
			"`dify_dataset_document_indexing_wait` instead. " +
			"Import by `dataset_id:document_id`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Document ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"dataset_id": schema.StringAttribute{
				MarkdownDescription: "The knowledge base this document belongs to. Changing this forces recreation.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Document name.",
				Required:            true,
			},
			"text": schema.StringAttribute{
				MarkdownDescription: "Document text content. Write-only and sensitive.",
				Optional:            true,
				Sensitive:           true,
			},
			"process_rule": schema.StringAttribute{
				MarkdownDescription: "Processing rules for chunking as a JSON string.",
				Optional:            true,
				Computed:            true,
			},
			"doc_form": schema.StringAttribute{
				MarkdownDescription: "`text_model` for standard text chunking, `hierarchical_model` for parent-child chunk structure, `qa_model` for question-answer pair extraction.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("text_model"),
			},
			"doc_language": schema.StringAttribute{
				MarkdownDescription: "Language of the document for processing optimization.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("English"),
			},
			"indexing_technique": schema.StringAttribute{
				MarkdownDescription: "`high_quality` uses embedding models for precise search; `economy` uses keyword-based indexing. Required when adding the first document to a knowledge base.",
				Optional:            true,
			},
			"retrieval_model": schema.StringAttribute{
				MarkdownDescription: "Retrieval model configuration as a JSON string.",
				Optional:            true,
			},
			"embedding_model": schema.StringAttribute{
				MarkdownDescription: "Embedding model name.",
				Optional:            true,
			},
			"embedding_model_provider": schema.StringAttribute{
				MarkdownDescription: "Embedding model provider.",
				Optional:            true,
			},
			"indexing_wait": schema.BoolAttribute{
				MarkdownDescription: "Whether Create/Update block until the document's indexing reaches `available`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"indexing_timeout": schema.Int64Attribute{
				MarkdownDescription: "Maximum seconds to wait for indexing to finish when `indexing_wait` is true.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"indexing_poll_interval": schema.Int64Attribute{
				MarkdownDescription: "Seconds between indexing status polls when `indexing_wait` is true.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3),
			},
			"indexing_status": schema.StringAttribute{
				MarkdownDescription: "Current document status (`queuing`, `indexing`, `paused`, `available`, `failed`, `disabled`, `archived`).",
				Computed:            true,
			},
			"indexing_error": schema.StringAttribute{
				MarkdownDescription: "Indexing error message when status is `failed`.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the document is enabled for retrieval.",
				Computed:            true,
			},
			"archived": schema.BoolAttribute{
				MarkdownDescription: "Whether the document is archived.",
				Computed:            true,
			},
			"created_at": schema.Int64Attribute{
				MarkdownDescription: "Unix seconds when the document was created.",
				Computed:            true,
			},
			"updated_at": schema.Int64Attribute{
				MarkdownDescription: "Unix seconds when the document was last updated.",
				Computed:            true,
			},
		},
	}
}

func (r *DocumentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// request converts the model to the create-by-text API payload.
func (m *DocumentResourceModel) request() DocumentCreateRequest {
	req := DocumentCreateRequest{
		Name: m.Name.ValueString(),
		Text: m.Text.ValueString(),
	}
	if known(m.ProcessRule) {
		raw := json.RawMessage(m.ProcessRule.ValueString())
		req.ProcessRule = &raw
	}
	if known(m.DocForm) {
		req.DocForm = stringPointer(m.DocForm.ValueString())
	}
	if known(m.DocLanguage) {
		req.DocLanguage = stringPointer(m.DocLanguage.ValueString())
	}
	if known(m.IndexingTechnique) {
		req.IndexingTechnique = stringPointer(m.IndexingTechnique.ValueString())
	}
	if known(m.RetrievalModel) {
		raw := json.RawMessage(m.RetrievalModel.ValueString())
		req.RetrievalModel = &raw
	}
	if known(m.EmbeddingModel) {
		req.EmbeddingModel = stringPointer(m.EmbeddingModel.ValueString())
	}
	if known(m.EmbeddingModelProvider) {
		req.EmbeddingModelProvider = stringPointer(m.EmbeddingModelProvider.ValueString())
	}
	return req
}

// updateRequest converts the model to the update-by-text API payload.
func (m *DocumentResourceModel) updateRequest() DocumentUpdateRequest {
	req := DocumentUpdateRequest{}
	if known(m.Name) {
		req.Name = stringPointer(m.Name.ValueString())
	}
	if known(m.Text) {
		req.Text = stringPointer(m.Text.ValueString())
	}
	if known(m.ProcessRule) {
		raw := json.RawMessage(m.ProcessRule.ValueString())
		req.ProcessRule = &raw
	}
	if known(m.DocForm) {
		req.DocForm = stringPointer(m.DocForm.ValueString())
	}
	if known(m.DocLanguage) {
		req.DocLanguage = stringPointer(m.DocLanguage.ValueString())
	}
	if known(m.RetrievalModel) {
		raw := json.RawMessage(m.RetrievalModel.ValueString())
		req.RetrievalModel = &raw
	}
	return req
}

// applyResult fills the model from a document response.
func (m *DocumentResourceModel) applyResult(result *Document) {
	m.ID = types.StringValue(result.ID)
	m.DatasetID = types.StringValue(result.DatasetID)
	m.Name = types.StringValue(result.Name)
	m.IndexingStatus = types.StringValue(indexingStatus(result.DisplayStatus))
	m.IndexingError = types.StringValue(result.Error)
	m.Enabled = types.BoolValue(result.Enabled)
	m.Archived = types.BoolValue(result.Archived)
	m.CreatedAt = types.Int64Value(result.CreatedAt)
	m.UpdatedAt = types.Int64Value(result.UpdatedAt)
	// The API always returns a process rule (its own serialization when
	// unconfigured); keep the configured value verbatim and only adopt the
	// server's when the attribute was not set (null or still unknown).
	if !known(m.ProcessRule) && len(result.DocumentProcessRule) > 0 {
		m.ProcessRule = types.StringValue(string(result.DocumentProcessRule))
	}
}

// splitImportID parses "dataset_id:document_id" from the import identifier.
func splitImportID(id string) (datasetID, documentID string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("import id must be in the form dataset_id:document_id, got %q", id)
	}
	return parts[0], parts[1], nil
}

func (r *DocumentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DocumentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateDocument(ctx, data.DatasetID.ValueString(), data.request())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create document: %s", err))
		return
	}

	// If indexing_wait is enabled, block until indexing reaches a terminal state.
	if data.IndexingWait.ValueBool() {
		timeout := time.Duration(data.IndexingTimeout.ValueInt64()) * time.Second
		interval := time.Duration(data.IndexingPollInterval.ValueInt64()) * time.Second

		doc, err := r.client.WaitForDocumentIndexing(ctx, data.DatasetID.ValueString(), result.Document.ID, timeout, interval)
		if err != nil {
			// Best-effort cleanup of the document we just created so it does not
			// become an orphan when apply is interrupted.
			_ = r.client.DeleteDocument(ctx, data.DatasetID.ValueString(), result.Document.ID)
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Document indexing did not complete: %s", err))
			return
		}
		data.applyResult(doc)
	} else {
		// Not waiting: still record the full create response so computed
		// attributes are known.
		data.applyResult(&result.Document)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DocumentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	doc, err := r.client.GetDocument(ctx, data.DatasetID.ValueString(), data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read document: %s", err))
		return
	}

	data.applyResult(doc)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DocumentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.UpdateDocument(ctx, data.DatasetID.ValueString(), data.ID.ValueString(), data.updateRequest())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update document: %s", err))
		return
	}

	if data.IndexingWait.ValueBool() {
		timeout := time.Duration(data.IndexingTimeout.ValueInt64()) * time.Second
		interval := time.Duration(data.IndexingPollInterval.ValueInt64()) * time.Second

		doc, err := r.client.WaitForDocumentIndexing(ctx, data.DatasetID.ValueString(), result.Document.ID, timeout, interval)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Document indexing did not complete after update: %s", err))
			return
		}
		data.applyResult(doc)
	} else {
		data.applyResult(&result.Document)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DocumentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DocumentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDocument(ctx, data.DatasetID.ValueString(), data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete document: %s", err))
		return
	}
}

func (r *DocumentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	datasetID, documentID, err := splitImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dataset_id"), datasetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), documentID)...)
}
