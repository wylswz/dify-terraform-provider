// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AppResource{}
var _ resource.ResourceWithImportState = &AppResource{}

func NewAppResource() resource.Resource {
	return &AppResource{}
}

// AppResource defines the resource implementation.
type AppResource struct {
	client *DifyClient
}

// AppResourceModel describes the resource data model.
type AppResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Mode            types.String `tfsdk:"mode"`
	Description     types.String `tfsdk:"description"`
	DSLYaml         types.String `tfsdk:"dsl_yaml"`
	ExportedDSLYaml types.String `tfsdk:"exported_dsl_yaml"`
	CreatorEmail    types.String `tfsdk:"creator_email"`
	EnableSite      types.Bool   `tfsdk:"enable_site"`
	EnableAPI       types.Bool   `tfsdk:"enable_api"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func (r *AppResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (r *AppResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Dify application via DSL YAML. The app lifecycle (create, update, delete) " +
			"is driven by the DSL content. Model provider credentials and plugins are managed as separate resources.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "App name. Overrides the name in DSL if set.",
				Optional:            true,
				Computed:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "App mode (e.g. `chat`, `workflow`, `agent-chat`). Computed from DSL.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "App description. Overrides DSL description if set.",
				Optional:            true,
				Computed:            true,
			},
			"dsl_yaml": schema.StringAttribute{
				MarkdownDescription: "DSL YAML content defining the app (desired state). " +
					"Changes to this field trigger an app update (re-import). " +
					"This value is preserved as-is and not overwritten by the exported DSL.",
				Required: true,
			},
			"exported_dsl_yaml": schema.StringAttribute{
				MarkdownDescription: "The DSL YAML exported from the live app (actual state). " +
					"May differ from dsl_yaml due to server-side overrides (name, description, etc).",
				Computed: true,
			},
			"creator_email": schema.StringAttribute{
				MarkdownDescription: "Email of the workspace member who will own the app. Required for create.",
				Required:            true,
			},
			"enable_site": schema.BoolAttribute{
				MarkdownDescription: "Whether the web app site is enabled.",
				Computed:            true,
			},
			"enable_api": schema.BoolAttribute{
				MarkdownDescription: "Whether the service API is enabled.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp.",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last update timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *AppResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	importReq := AppCreateRequest{
		YAMLContent:  data.DSLYaml.ValueString(),
		CreatorEmail: data.CreatorEmail.ValueString(),
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueString(),
	}

	result, err := r.client.CreateApp(ctx, importReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create app: %s", err))
		return
	}

	// Read back the full app detail
	app, err := r.client.GetApp(ctx, result.AppID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read created app: %s", err))
		return
	}

	data.ID = types.StringValue(app.ID)
	data.Name = types.StringValue(app.Name)
	data.Mode = types.StringValue(app.Mode)
	data.Description = types.StringValue(app.Description)
	data.EnableSite = types.BoolValue(app.EnableSite)
	data.EnableAPI = types.BoolValue(app.EnableAPI)
	data.ExportedDSLYaml = types.StringValue(app.DSLYaml)
	data.CreatedAt = types.StringValue(app.CreatedAt)
	data.UpdatedAt = types.StringValue(app.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.GetApp(ctx, data.ID.ValueString())
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read app: %s", err))
		return
	}

	data.ID = types.StringValue(app.ID)
	data.Name = types.StringValue(app.Name)
	data.Mode = types.StringValue(app.Mode)
	data.Description = types.StringValue(app.Description)
	data.EnableSite = types.BoolValue(app.EnableSite)
	data.EnableAPI = types.BoolValue(app.EnableAPI)
	data.ExportedDSLYaml = types.StringValue(app.DSLYaml)
	data.CreatedAt = types.StringValue(app.CreatedAt)
	data.UpdatedAt = types.StringValue(app.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := AppUpdateRequest{
		YAMLContent: data.DSLYaml.ValueString(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	}

	_, err := r.client.UpdateApp(ctx, data.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update app: %s", err))
		return
	}

	// Read back the updated app
	app, err := r.client.GetApp(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read updated app: %s", err))
		return
	}

	data.Name = types.StringValue(app.Name)
	data.Mode = types.StringValue(app.Mode)
	data.Description = types.StringValue(app.Description)
	data.EnableSite = types.BoolValue(app.EnableSite)
	data.EnableAPI = types.BoolValue(app.EnableAPI)
	data.ExportedDSLYaml = types.StringValue(app.DSLYaml)
	data.UpdatedAt = types.StringValue(app.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteApp(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete app: %s", err))
		return
	}
}

func (r *AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
