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
	Active       types.Bool   `tfsdk:"active"`
}

func (r *ToolProviderCredentialResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tool_provider_credential"
}

func (r *ToolProviderCredentialResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a builtin tool provider credential in Dify. " +
			"`provider_name` is the provider path such as `langgenius/tavily/tavily`. " +
			"Credential values are write-only: the API never returns them, so state keeps the configured values.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Tool provider id (e.g. `langgenius/tavily/tavily`).",
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
				MarkdownDescription: "Human-readable name for this credential (max 30 characters).",
				Optional:            true,
				Computed:            true,
			},
			"credentials": schema.MapAttribute{
				MarkdownDescription: "Credential key-value pairs keyed by the provider's credential form fields " +
					"(e.g. `tavily_api_key`). Write-only and sensitive.",
				ElementType: types.StringType,
				Required:    true,
				Sensitive:   true,
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the provider's default credential.",
				Computed:            true,
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

	res, err := r.client.CreateToolCredential(ctx, data.ProviderName.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create tool credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desc, err := r.client.DescribeToolProvider(ctx, data.ProviderName.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to describe tool provider: %s", err))
		return
	}

	credentialID := data.CredentialID.ValueString()
	creds := credentialIDs(desc.Credentials)
	ref, ok := creds[credentialID]
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	if ref.Name != nil {
		data.Name = types.StringValue(*ref.Name)
	}
	data.Active = types.BoolValue(activeCredentialID(desc) == credentialID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.UpdateToolCredential(ctx, data.ProviderName.ValueString(), data.CredentialID.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update tool credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ToolProviderCredentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ToolProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteToolCredential(ctx, data.ProviderName.ValueString(), data.CredentialID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete tool credential: %s", err))
		return
	}
}
