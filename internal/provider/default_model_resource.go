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

var _ resource.Resource = &DefaultModelResource{}

func NewDefaultModelResource() resource.Resource {
	return &DefaultModelResource{}
}

// DefaultModelResource defines the resource implementation.
type DefaultModelResource struct {
	client *DifyClient
}

// DefaultModelResourceModel describes the resource data model.
type DefaultModelResourceModel struct {
	ModelType    types.String `tfsdk:"model_type"`
	ProviderName types.String `tfsdk:"provider_name"`
	Model        types.String `tfsdk:"model"`
}

func (r *DefaultModelResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_default_model"
}

func (r *DefaultModelResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the workspace's default model of one model type " +
			"(`llm`, `text-embedding`, `rerank`, `speech2text`, `tts`, `moderation`). " +
			"The API has no way to unset a default, so destroy only removes this " +
			"resource from state — the configured default stays in place on the server.",

		Attributes: map[string]schema.Attribute{
			"model_type": schema.StringAttribute{
				MarkdownDescription: "Model type the default applies to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Provider id of the default model (e.g. `langgenius/openai/openai`).",
				Required:            true,
			},
			"model": schema.StringAttribute{
				MarkdownDescription: "Model name used as the default.",
				Required:            true,
			},
		},
	}
}

func (r *DefaultModelResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DefaultModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DefaultModelResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SetDefaultModel(ctx, data.ModelType.ValueString(), data.ProviderName.ValueString(), data.Model.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set default model: %s", err))
		return
	}

	data.ProviderName = types.StringPointerValue(result.Provider)
	data.Model = types.StringPointerValue(result.Model)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DefaultModelResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.GetDefaultModel(ctx, data.ModelType.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read default model: %s", err))
		return
	}

	data.ProviderName = types.StringPointerValue(result.Provider)
	data.Model = types.StringPointerValue(result.Model)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DefaultModelResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SetDefaultModel(ctx, data.ModelType.ValueString(), data.ProviderName.ValueString(), data.Model.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to set default model: %s", err))
		return
	}

	data.ProviderName = types.StringPointerValue(result.Provider)
	data.Model = types.StringPointerValue(result.Model)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// The API has no "unset default" operation: destroying only removes the
	// resource from Terraform state, as the schema description states.
}
