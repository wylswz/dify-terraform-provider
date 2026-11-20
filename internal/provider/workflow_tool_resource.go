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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &WorkflowToolResource{}
var _ resource.ResourceWithImportState = &WorkflowToolResource{}

func NewWorkflowToolResource() resource.Resource {
	return &WorkflowToolResource{}
}

// WorkflowToolResource defines the resource implementation.
type WorkflowToolResource struct {
	client *DifyClient
}

// WorkflowToolIconModel describes the tool's emoji tile.
type WorkflowToolIconModel struct {
	Content    types.String `tfsdk:"content"`
	Background types.String `tfsdk:"background"`
}

// WorkflowToolParameterModel describes one parameter of a workflow tool.
type WorkflowToolParameterModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Form        types.String `tfsdk:"form"`
}

// WorkflowToolResourceModel describes the resource data model.
type WorkflowToolResourceModel struct {
	ID            types.String                 `tfsdk:"id"`
	WorkflowAppID types.String                 `tfsdk:"workflow_app_id"`
	Name          types.String                 `tfsdk:"name"`
	Label         types.String                 `tfsdk:"label"`
	Description   types.String                 `tfsdk:"description"`
	Icon          *WorkflowToolIconModel       `tfsdk:"icon"`
	Parameters    []WorkflowToolParameterModel `tfsdk:"parameters"`
	PrivacyPolicy types.String                 `tfsdk:"privacy_policy"`
	Labels        []types.String               `tfsdk:"labels"`
	Synced        types.Bool                   `tfsdk:"synced"`
}

func (r *WorkflowToolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow_tool"
}

func (r *WorkflowToolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a workflow tool — a workflow app published as a tool provider. " +
			"The workflow app must be published for the tool to work; set `publish = true` on the " +
			"backing `dify_app`. Import by id: `terraform import dify_workflow_tool.x <id>`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Workflow tool ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"workflow_app_id": schema.StringAttribute{
				MarkdownDescription: "The workflow app this tool publishes. The app must have a published " +
					"workflow — set `publish = true` on its `dify_app` resource.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Tool name (alphanumeric; also the identifier callers see).",
				Required:            true,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "Human-readable tool label.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Tool description shown to agents picking tools.",
				Required:            true,
			},
			"icon": schema.SingleNestedAttribute{
				MarkdownDescription: "Emoji tile the workspace shows for the tool.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"content": schema.StringAttribute{
						MarkdownDescription: "Emoji content.",
						Required:            true,
					},
					"background": schema.StringAttribute{
						MarkdownDescription: "Tile background color.",
						Required:            true,
					},
				},
			},
			"parameters": schema.ListNestedAttribute{
				MarkdownDescription: "Parameters the tool accepts.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Parameter name.",
							Required:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Parameter description.",
							Required:            true,
						},
						"form": schema.StringAttribute{
							MarkdownDescription: "Parameter form (`llm` fills it from the model, `form` from node config).",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("llm"),
						},
					},
				},
			},
			"privacy_policy": schema.StringAttribute{
				MarkdownDescription: "Privacy policy URL.",
				Optional:            true,
			},
			"labels": schema.ListAttribute{
				MarkdownDescription: "Tool labels.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"synced": schema.BoolAttribute{
				MarkdownDescription: "False once the app's published workflow moved past this tool's version.",
				Computed:            true,
			},
		},
	}
}

func (r *WorkflowToolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// request converts the model to the API payload. includeAppID reports whether
// workflow_app_id travels in the body (create only; update keeps the app).
func (m *WorkflowToolResourceModel) request(includeAppID bool) workflowToolRequest {
	req := workflowToolRequest{
		Name:          m.Name.ValueString(),
		Label:         m.Label.ValueString(),
		Description:   m.Description.ValueString(),
		PrivacyPolicy: m.PrivacyPolicy.ValueStringPointer(),
		Labels:        stringValues(m.Labels),
		Parameters:    make([]WorkflowToolParameter, 0, len(m.Parameters)),
	}
	if includeAppID {
		req.WorkflowAppID = m.WorkflowAppID.ValueString()
	}
	if m.Icon != nil {
		req.Icon = map[string]string{
			"content":    m.Icon.Content.ValueString(),
			"background": m.Icon.Background.ValueString(),
		}
	}
	for _, p := range m.Parameters {
		req.Parameters = append(req.Parameters, WorkflowToolParameter{
			Name:        p.Name.ValueString(),
			Description: p.Description.ValueString(),
			Form:        p.Form.ValueString(),
		})
	}
	return req
}

// applyResult fills the model from a describe/write response.
func (m *WorkflowToolResourceModel) applyResult(result *WorkflowTool) {
	m.ID = types.StringValue(result.WorkflowToolID)
	m.WorkflowAppID = types.StringValue(result.WorkflowAppID)
	m.Name = types.StringValue(result.Name)
	m.Label = types.StringValue(result.Label)
	m.Description = types.StringValue(result.Description)
	m.Icon = &WorkflowToolIconModel{
		Content:    types.StringValue(result.Icon["content"]),
		Background: types.StringValue(result.Icon["background"]),
	}
	m.PrivacyPolicy = optionalString(result.PrivacyPolicy)
	m.Synced = types.BoolValue(result.Synced)

	// parameters is Optional-only, so an unconfigured null must stay null even
	// when the server derives entries; configured values store the echo.
	if m.Parameters != nil && len(result.Parameters) > 0 {
		params := make([]WorkflowToolParameterModel, 0, len(result.Parameters))
		for _, p := range result.Parameters {
			params = append(params, WorkflowToolParameterModel{
				Name:        types.StringValue(p.Name),
				Description: types.StringValue(p.Description),
				Form:        types.StringValue(p.Form),
			})
		}
		m.Parameters = params
	}
}

func (r *WorkflowToolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WorkflowToolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateWorkflowTool(ctx, data.request(true))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create workflow tool: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowToolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WorkflowToolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.GetWorkflowTool(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow tool: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowToolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WorkflowToolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.UpdateWorkflowTool(ctx, data.ID.ValueString(), data.request(false))
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update workflow tool: %s", err))
		return
	}

	data.applyResult(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowToolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WorkflowToolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWorkflowTool(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete workflow tool: %s", err))
		return
	}
}

func (r *WorkflowToolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
