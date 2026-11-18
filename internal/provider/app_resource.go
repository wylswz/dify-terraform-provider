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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"gopkg.in/yaml.v3"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AppResource{}
var _ resource.ResourceWithImportState = &AppResource{}
var _ resource.ResourceWithModifyPlan = &AppResource{}

func NewAppResource() resource.Resource {
	return &AppResource{}
}

// AppResource defines the resource implementation.
type AppResource struct {
	client *DifyClient
}

// AppResourceModel describes the resource data model.
type AppResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Mode              types.String `tfsdk:"mode"`
	Description       types.String `tfsdk:"description"`
	DSLYaml           types.String `tfsdk:"dsl_yaml"`
	ExportedDSLYaml   types.String `tfsdk:"exported_dsl_yaml"`
	DraftHash         types.String `tfsdk:"draft_hash"`
	Publish           types.Bool   `tfsdk:"publish"`
	ServiceAPIEnabled types.Bool   `tfsdk:"service_api_enabled"`
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

// appModesWithDrafts are the modes the publish op serves (the server's
// GRAPH_MODES). Other modes have no draft and go live on import.
var appModesWithDrafts = appModesSupportingOverwrite

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
			"publish": schema.BoolAttribute{
				MarkdownDescription: "Publish the imported draft as a new version and make the app use it " +
					"(POST /apps/{id}:publish) after every create/update import. Leave false to keep the " +
					"app on its last published version; the draft is still imported either way. " +
					"Only `workflow` and `advanced-chat` apps have drafts — other modes are live as soon " +
					"as the import lands, so `publish` is ignored for them.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"service_api_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the app's Service API is on (PATCH /apps/{id}/service-api, " +
					"or /service-api/agent when mode is `agent`). When set, create/update apply it after " +
					"the import; read refreshes it from the server. **Warning**: this server's app " +
					"management calls (DSL export, publish, versions) require the Service API — with " +
					"`service_api_enabled = false`, Read/Update of this resource fail.",
				Optional: true,
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

// ModifyPlan warns when publish is requested for a mode the publish op does
// not serve. mode is computed, so the warning fires only once state knows it;
// on create of a non-graph mode the call is skipped silently.
func (r *AppResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var plan, state AppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.Publish.ValueBool() && state.Mode.ValueString() != "" &&
		!appModesWithDrafts[state.Mode.ValueString()] {
		resp.Diagnostics.AddWarning(
			"publish is ignored for this app mode",
			fmt.Sprintf("Only workflow and advanced-chat apps have drafts; mode %q is live "+
				"as soon as the import lands, so POST /apps/{id}:publish is skipped.",
				state.Mode.ValueString()),
		)
	}
}

// applyPostImport runs the lifecycle calls that follow a DSL import: publish
// when requested and the app has drafts, the configured Service API toggle,
// and a service-api read to fill service_api_enabled. modeAgent reports
// whether the app uses the /service-api/agent endpoints (mode `agent`); any
// other mode uses the regular ones.
func (r *AppResource) applyPostImport(ctx context.Context, data *AppResourceModel, configuredServiceAPI bool) error {
	appID := data.ID.ValueString()

	if data.Publish.ValueBool() && appModesWithDrafts[data.Mode.ValueString()] {
		if _, err := r.client.PublishApp(ctx, appID, "", ""); err != nil {
			return fmt.Errorf("unable to publish app: %s", err)
		}
	}

	modeAgent := data.Mode.ValueString() == "agent"
	if configuredServiceAPI {
		if _, err := r.client.SetServiceAPI(ctx, appID, modeAgent, data.ServiceAPIEnabled.ValueBool()); err != nil {
			return fmt.Errorf("unable to set app Service API: %s", err)
		}
	}

	serviceAPI, err := r.client.GetServiceAPI(ctx, appID, modeAgent)
	if err != nil {
		return fmt.Errorf("unable to read app Service API: %s", err)
	}
	data.ServiceAPIEnabled = types.BoolValue(serviceAPI.Enabled)

	return nil
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

	if err := r.applyPostImport(ctx, &data, !cfg.ServiceAPIEnabled.IsNull()); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

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
		if isServiceAPIDisabled(err) {
			resp.Diagnostics.AddError(
				"App Service API Disabled",
				"The server requires the app's Service API for DSL export and other management calls, "+
					"but it is turned off for this app. Re-enable it via the console or set "+
					"service_api_enabled = true before reading or updating this resource.")
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to export app DSL: %s", err))
		return
	}

	applyExport(&data, export, readOverrideFlags(ctx, req.Private))

	serviceAPI, err := r.client.GetServiceAPI(ctx, data.ID.ValueString(), data.Mode.ValueString() == "agent")
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read app Service API: %s", err))
		return
	}
	data.ServiceAPIEnabled = types.BoolValue(serviceAPI.Enabled)

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

	if err := r.applyPostImport(ctx, &data, !cfg.ServiceAPIEnabled.IsNull()); err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}

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
