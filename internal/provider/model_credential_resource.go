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

var _ resource.Resource = &ModelCredentialResource{}

func NewModelCredentialResource() resource.Resource {
	return &ModelCredentialResource{}
}

// ModelCredentialResource defines the resource implementation.
type ModelCredentialResource struct {
	client *DifyClient
}

// ModelCredentialResourceModel describes the resource data model.
type ModelCredentialResourceModel struct {
	ProviderName types.String `tfsdk:"provider_name"`
	Model        types.String `tfsdk:"model"`
	ModelType    types.String `tfsdk:"model_type"`
	CredentialID types.String `tfsdk:"credential_id"`
	Name         types.String `tfsdk:"name"`
	Credentials  types.Map    `tfsdk:"credentials"`
	MakeActive   types.Bool   `tfsdk:"make_active"`
	Active       types.Bool   `tfsdk:"active"`
}

func (r *ModelCredentialResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_model_credential"
}

func (r *ModelCredentialResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a credential of one custom model of a model provider " +
			"(e.g. an OpenAI-compatible or Ollama model). The first credential of a model not yet " +
			"set up becomes its active credential. Credential values are write-only: the API never " +
			"returns them, so state keeps the configured values.",

		Attributes: map[string]schema.Attribute{
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Model provider id (e.g. `langgenius/openai/openai`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"model": schema.StringAttribute{
				MarkdownDescription: "Model name the credential belongs to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"model_type": schema.StringAttribute{
				MarkdownDescription: "Model type (`llm`, `text-embedding`, `rerank`, `speech2text`, `tts`, `moderation`).",
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
				MarkdownDescription: "Credential key-value pairs keyed by the provider's custom-model form fields. " +
					"Write-only and sensitive.",
				ElementType: types.StringType,
				Required:    true,
				Sensitive:   true,
			},
			"make_active": schema.BoolAttribute{
				MarkdownDescription: "Make this credential the model's active one after every create/update " +
					"(POST .../models/credentials/{credential_id}:activate).",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the model's active credential.",
				Computed:            true,
			},
		},
	}
}

func (r *ModelCredentialResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ModelCredentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ModelCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.CreateModelScopedCredential(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create model credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	if data.MakeActive.ValueBool() && !data.Active.ValueBool() {
		res, err = r.client.ActivateModelScopedCredential(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), data.CredentialID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate model credential: %s", err))
			return
		}
		applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelCredentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ModelCredentialResourceModel

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
	for _, custom := range desc.CustomModels {
		if custom.Model != data.Model.ValueString() || custom.ModelType != data.ModelType.ValueString() {
			continue
		}
		if custom.ActiveCredential != nil && custom.ActiveCredential.ID == credentialID {
			data.Active = types.BoolValue(true)
			if custom.ActiveCredential.Name != nil {
				data.Name = types.StringValue(*custom.ActiveCredential.Name)
			}
		} else {
			data.Active = types.BoolValue(false)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	// The model itself is gone, so its credentials are gone too.
	resp.State.RemoveResource(ctx)
}

func (r *ModelCredentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ModelCredentialResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.UpdateModelScopedCredential(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), data.CredentialID.ValueString(), credentialsToMap(data.Credentials), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update model credential: %s", err))
		return
	}

	applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)

	if data.MakeActive.ValueBool() && !data.Active.ValueBool() {
		res, err = r.client.ActivateModelScopedCredential(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), data.CredentialID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to activate model credential: %s", err))
			return
		}
		applyCredentialWrite(&data.CredentialID, &data.Name, &data.Active, res)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ModelCredentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ModelCredentialResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteModelScopedCredential(ctx, data.ProviderName.ValueString(), data.Model.ValueString(), data.ModelType.ValueString(), data.CredentialID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete model credential: %s", err))
		return
	}
}
