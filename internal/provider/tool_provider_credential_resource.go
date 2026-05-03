// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ToolProviderCredentialResource{}

func NewToolProviderCredentialResource() resource.Resource {
	return &ToolProviderCredentialResource{}
}

// ToolProviderCredentialResource defines the resource implementation.
type ToolProviderCredentialResource struct {
	client *DifyClient
}

// ToolProviderCredentialResourceModel describes the resource data model.
type ToolProviderCredentialResourceModel struct {
	ProviderName types.String `tfsdk:"provider_name"`
	CredentialID types.String `tfsdk:"credential_id"`
	Name         types.String `tfsdk:"name"`
	Credentials  types.Map    `tfsdk:"credentials"`
}

func (r *ToolProviderCredentialResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tool_provider_credential"
}

func (r *ToolProviderCredentialResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a builtin tool provider credential in Dify. " +
			"Credentials are key-value pairs (e.g. `api_key`) for a specific tool provider.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Tool provider name (e.g. `tavily`, `google`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"credential_id": schema.StringAttribute{
				MarkdownDescription: "Credential ID (assigned by Dify after creation).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable name for this credential.",
				Optional:            true,
			},
			"credentials": schema.MapAttribute{
				MarkdownDescription: "Credential key-value pairs (e.g. `api_key = \"sk-...\"`). " +
					"Values are treated as sensitive.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *ToolProviderCredentialResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ToolProviderCredentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert types.Map to map[string]interface{}
	credMap := make(map[string]interface{})
	for k, v := range data.Credentials.Elements() {
		credMap[k] = v.(types.String).ValueString()
	}

	createReq := CredentialCreateRequest{
		Credentials: credMap,
		Name:        data.Name.ValueString(),
	}

	createResp, err := r.client.CreateToolCredential(ctx, data.ProviderName.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create tool credential: %s", err))
		return
	}

	if createResp.CredentialID != "" {
		data.CredentialID = types.StringValue(createResp.CredentialID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	credResp, err := r.client.GetToolCredential(ctx, data.ProviderName.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read tool credential: %s", err))
		return
	}

	// Update credential_id from response if available
	if credResp.CredentialID != "" {
		data.CredentialID = types.StringValue(credResp.CredentialID)
	}

	// Note: we don't overwrite credentials from the API response since
	// the API masks sensitive values. We keep the TF state values.

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	credMap := make(map[string]interface{})
	for k, v := range data.Credentials.Elements() {
		credMap[k] = v.(types.String).ValueString()
	}

	updateReq := CredentialUpdateRequest{
		CredentialID: data.CredentialID.ValueString(),
		Credentials:  credMap,
		Name:         data.Name.ValueString(),
	}

	err := r.client.UpdateToolCredential(ctx, data.ProviderName.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update tool credential: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteReq := CredentialDeleteRequest{
		CredentialID: data.CredentialID.ValueString(),
	}

	err := r.client.DeleteToolCredential(ctx, data.ProviderName.ValueString(), deleteReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete tool credential: %s", err))
		return
	}
}
