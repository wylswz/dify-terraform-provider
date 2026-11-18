// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ModelProviderCredentialResource{}

func NewModelProviderCredentialResource() resource.Resource {
	return &ModelProviderCredentialResource{}
}

// ModelProviderCredentialResource defines the resource implementation.
type ModelProviderCredentialResource struct {
	client *DifyClient
}

// ModelProviderCredentialResourceModel describes the resource data model.
type ModelProviderCredentialResourceModel struct {
	ProviderName types.String `tfsdk:"provider_name"`
	CredentialID types.String `tfsdk:"credential_id"`
	Name         types.String `tfsdk:"name"`
	Credentials  types.Map    `tfsdk:"credentials"`
	MakeActive   types.Bool   `tfsdk:"make_active"`
	Active       types.Bool   `tfsdk:"active"`
}

// credentialsToMap converts the credentials map attribute to a plain map for the API.
func credentialsToMap(m types.Map) map[string]any {
	out := make(map[string]any, len(m.Elements()))
	for k, v := range m.Elements() {
		if s, ok := v.(types.String); ok {
			out[k] = s.ValueString()
		}
	}
	return out
}

// applyCredentialWrite records the server-assigned id, name and active flag
// from a credential write response. Credential secret values are write-only:
// the API never returns them, so state keeps the configured values.
// Shared by the model and tool provider credential resources.
func applyCredentialWrite(credentialID *types.String, name *types.String, active *types.Bool, res *CredentialWriteResult) {
	*credentialID = types.StringValue(res.ID)
	if res.Name != nil && *res.Name != "" {
		*name = types.StringValue(*res.Name)
	}
	*active = types.BoolValue(res.Active)
}

func (r *ModelProviderCredentialResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_model_provider_credential"
}

func (r *ModelProviderCredentialResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a model provider credential in Dify. " +
			"`provider_name` is the provider path such as `langgenius/openai/openai`. " +
			"Credential values are write-only: the API never returns them, so state keeps the configured values.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Model provider id (e.g. `langgenius/openai/openai`).",
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
				Computed:            true,
			},
			"credentials": schema.MapAttribute{
				MarkdownDescription: "Credential key-value pairs keyed by the provider's credential form fields " +
					"(e.g. `openai_api_key`). Write-only and sensitive.",
				ElementType: types.StringType,
				Required:    true,
				Sensitive:   true,
			},
			"make_active": schema.BoolAttribute{
				MarkdownDescription: "Make this credential the provider's active one after every create/update " +
					"(POST .../credentials/{credential_id}:activate).",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the provider's active credential.",
				Computed:            true,
			},
		},
	}
}

func (r *ModelProviderCredentialResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ModelProviderCredentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ModelProviderCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.CreateModelCredential(ctx, data.ProviderName.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	if data.MakeActive.ValueBool() && !data.Active.ValueBool() {
		res, err = r.client.ActivateModelProviderCredential(ctx, data.ProviderName.ValueString(), data.CredentialID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate credential: %s", err))
			return
		}
		applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelProviderCredentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ModelProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desc, err := r.client.DescribeModelProvider(ctx, data.ProviderName.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to describe model provider: %s", err))
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

func (r *ModelProviderCredentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ModelProviderCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.UpdateModelCredential(ctx, data.ProviderName.ValueString(), data.CredentialID.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelProviderCredentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ModelProviderCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteModelCredential(ctx, data.ProviderName.ValueString(), data.CredentialID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete credential: %s", err))
		return
	}
}
