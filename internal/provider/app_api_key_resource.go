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

var _ resource.Resource = &AppAPIKeyResource{}

func NewAppAPIKeyResource() resource.Resource {
	return &AppAPIKeyResource{}
}

// AppAPIKeyResource defines the resource implementation.
type AppAPIKeyResource struct {
	client *DifyClient
}

// AppAPIKeyResourceModel describes the resource data model.
type AppAPIKeyResourceModel struct {
	AppID     types.String `tfsdk:"app_id"`
	KeyID     types.String `tfsdk:"key_id"`
	Token     types.String `tfsdk:"token"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *AppAPIKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_api_key"
}

func (r *AppAPIKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an API key for a Dify application.",

		Attributes: map[string]schema.Attribute{
			"app_id": schema.StringAttribute{
				MarkdownDescription: "The app ID this API key belongs to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key_id": schema.StringAttribute{
				MarkdownDescription: "API key ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "The API key token (e.g. `app-xxxxxxxx`). Sensitive.",
				Computed:            true,
				Sensitive:           true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *AppAPIKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AppAPIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AppAPIKeyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.CreateAppAPIKey(ctx, data.AppID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create API key: %s", err))
		return
	}

	data.KeyID = types.StringValue(key.ID)
	data.Token = types.StringValue(key.Token)
	data.CreatedAt = types.StringValue(key.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppAPIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AppAPIKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keys, err := r.client.ListAppAPIKeys(ctx, data.AppID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list API keys: %s", err))
		return
	}

	found := false
	keyID := data.KeyID.ValueString()
	for _, k := range keys.Data {
		if k.ID == keyID {
			data.Token = types.StringValue(k.Token)
			data.CreatedAt = types.StringValue(k.CreatedAt)
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppAPIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// API keys are immutable — no update possible. All fields use RequiresReplace.
	var data AppAPIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppAPIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AppAPIKeyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteAppAPIKey(ctx, data.AppID.ValueString(), data.KeyID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete API key: %s", err))
		return
	}
}
