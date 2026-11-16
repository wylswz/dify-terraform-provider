// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"gopkg.in/yaml.v3"
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
	DraftHash       types.String `tfsdk:"draft_hash"`
}

// dslAppMeta is the top-level `app:` map of an exported DSL document.
type dslAppMeta struct {
	App struct {
		Name        string `yaml:"name"`
		Mode        string `yaml:"mode"`
		Description string `yaml:"description"`
	} `yaml:"app"`
}

// parseDSLAppMeta extracts name/mode/description from exported DSL YAML.
// A document without an `app:` map yields zero values.
func parseDSLAppMeta(yamlContent string) (dslAppMeta, error) {
	var meta dslAppMeta
	err := yaml.Unmarshal([]byte(yamlContent), &meta)
	return meta, err
}

// attrOverridesPrivateKey stores which of name/description the user configured,
// so Read can tell configured overrides apart from export-derived values.
const attrOverridesPrivateKey = "attr_overrides"

// appModesSupportingOverwrite are the modes the server accepts for DSL overwrite
// imports; any other mode requires replacing the app.
var appModesSupportingOverwrite = map[string]bool{
	"workflow":      true,
	"advanced-chat": true,
}

// requireReplaceForNonWorkflowApps forces replacement when the prior state's app
// mode cannot be overwritten via DSL import. The server only overwrites
// workflow and advanced-chat apps, so for every other mode a changed attribute
// can only take effect through delete + create.
func requireReplaceForNonWorkflowApps(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	var state AppResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !appModesSupportingOverwrite[state.Mode.ValueString()] && state.Mode.ValueString() != "" {
		resp.RequiresReplace = true
	}
}

func (r *AppResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (r *AppResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Dify application via DSL YAML. The app lifecycle (create, update, delete) " +
			"is driven by the DSL content imported through the workspace OpenAPI. " +
			"Only `workflow` and `advanced-chat` apps support in-place DSL overwrite; " +
			"changes to apps of any other mode require replacement.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID (assigned by Dify).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"dsl_yaml": schema.StringAttribute{
				MarkdownDescription: "DSL YAML content defining the app (desired state). " +
					"Changes trigger a DSL re-import; for non-workflow app modes this forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						requireReplaceForNonWorkflowApps,
						"Replace app when DSL changes on a non-workflow mode",
						"The Dify OpenAPI only overwrites `workflow` and `advanced-chat` apps; "+
							"for other modes a DSL change requires replacing the app.",
					),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "App name. Overrides the name in the DSL when set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						requireReplaceForNonWorkflowApps,
						"Replace app when name changes on a non-workflow mode",
						"The Dify OpenAPI only overwrites `workflow` and `advanced-chat` apps; "+
							"for other modes a name change requires replacing the app.",
					),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "App description. Overrides the DSL description when set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						requireReplaceForNonWorkflowApps,
						"Replace app when description changes on a non-workflow mode",
						"The Dify OpenAPI only overwrites `workflow` and `advanced-chat` apps; "+
							"for other modes a description change requires replacing the app.",
					),
				},
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "App mode (e.g. `chat`, `workflow`, `agent-chat`), read from the exported DSL.",
				Computed:            true,
			},
			"exported_dsl_yaml": schema.StringAttribute{
				MarkdownDescription: "The DSL YAML exported from the live app (actual state). " +
					"May differ from dsl_yaml due to server-side normalization.",
				Computed: true,
			},
			"draft_hash": schema.StringAttribute{
				MarkdownDescription: "Hash of the app's current draft, returned by the DSL export. " +
					"Passed back on overwrite imports so concurrent draft edits are refused.",
				Computed: true,
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

// applyExport fills the computed attributes from a DSL export. name/description
// are only taken from the export when the user did not configure them
// (overrides), which keeps configured values stable and avoids perpetual diffs.
func applyExport(data *AppResourceModel, export *AppExport, overrides map[string]bool) {
	data.ExportedDSLYaml = types.StringValue(export.Data)
	if export.DraftHash == "" {
		data.DraftHash = types.StringNull()
	} else {
		data.DraftHash = types.StringValue(export.DraftHash)
	}

	meta, err := parseDSLAppMeta(export.Data)
	if err != nil {
		return
	}

	if meta.App.Mode != "" {
		data.Mode = types.StringValue(meta.App.Mode)
	}
	if !overrides["name"] {
		data.Name = types.StringValue(meta.App.Name)
	}
	if !overrides["description"] {
		data.Description = types.StringValue(meta.App.Description)
	}
}

// privateStateReader/Writer are the privatestate.ProviderData methods the
// resource needs, expressed as interfaces so helpers need not name the
// framework-internal privatestate type.
type privateStateReader interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type privateStateWriter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// configOverrides reports which of name/description are set in configuration.
func configOverrides(cfg AppResourceModel) map[string]bool {
	return map[string]bool{
		"name":        !cfg.Name.IsNull(),
		"description": !cfg.Description.IsNull(),
	}
}

// readOverrideFlags loads the configured name/description flags from private
// state; absent or undecodable data means nothing is configured.
func readOverrideFlags(ctx context.Context, private privateStateReader) map[string]bool {
	overrides := map[string]bool{}
	if private == nil {
		return overrides
	}
	raw, diags := private.GetKey(ctx, attrOverridesPrivateKey)
	if diags.HasError() || len(raw) == 0 {
		return overrides
	}
	_ = json.Unmarshal(raw, &overrides)
	return overrides
}

// writeOverrideFlags records the configured name/description flags in private
// state so Read can distinguish them from export-derived values.
func writeOverrideFlags(ctx context.Context, private privateStateWriter, overrides map[string]bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if private == nil {
		return diags
	}
	raw, err := json.Marshal(overrides)
	if err != nil {
		diags.AddError("Error Encoding Private State", err.Error())
		return diags
	}
	return private.SetKey(ctx, attrOverridesPrivateKey, raw)
}

func (r *AppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AppResourceModel
	var cfg AppResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	overrides := configOverrides(cfg)

	result, err := r.client.ImportApp(ctx, AppImportRequest{
		YAMLContent: data.DSLYaml.ValueString(),
		Name:        cfg.Name.ValueString(),
		Description: cfg.Description.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create app: %s", err))
		return
	}

	export, err := r.client.ExportAppDSL(ctx, result.AppID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to export created app DSL: %s", err))
		return
	}

	data.ID = types.StringValue(result.AppID)
	applyExport(&data, export, overrides)
	if resp.Private != nil {
		resp.Diagnostics.Append(writeOverrideFlags(ctx, resp.Private, overrides)...)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	export, err := r.client.ExportAppDSL(ctx, data.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to export app DSL: %s", err))
		return
	}

	applyExport(&data, export, readOverrideFlags(ctx, req.Private))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AppResourceModel
	var cfg AppResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	overrides := configOverrides(cfg)

	_, err := r.client.ImportApp(ctx, AppImportRequest{
		YAMLContent: data.DSLYaml.ValueString(),
		Name:        cfg.Name.ValueString(),
		Description: cfg.Description.ValueString(),
		AppID:       data.ID.ValueString(),
		DraftHash:   data.DraftHash.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update app: %s", err))
		return
	}

	export, err := r.client.ExportAppDSL(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to export updated app DSL: %s", err))
		return
	}

	applyExport(&data, export, overrides)
	if resp.Private != nil {
		resp.Diagnostics.Append(writeOverrideFlags(ctx, resp.Private, overrides)...)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AppResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteApp(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete app: %s", err))
		return
	}
}

func (r *AppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
