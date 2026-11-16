// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &PluginInstallResource{}

func NewPluginInstallResource() resource.Resource {
	return &PluginInstallResource{}
}

// PluginInstallResource defines the resource implementation.
type PluginInstallResource struct {
	client *DifyClient
}

// PluginInstallResourceModel describes the resource data model.
type PluginInstallResourceModel struct {
	PluginUniqueIdentifier types.String `tfsdk:"plugin_unique_identifier"`
	PluginID               types.String `tfsdk:"plugin_id"`
	Version                types.String `tfsdk:"version"`
	Category               types.String `tfsdk:"category"`
}

func (r *PluginInstallResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_install"
}

func (r *PluginInstallResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a plugin installation in the workspace. " +
			"Only marketplace identifiers are supported (e.g. `langgenius/openai:0.3.8@<sha256>`).",

		Attributes: map[string]schema.Attribute{
			"plugin_unique_identifier": schema.StringAttribute{
				MarkdownDescription: "Versioned plugin identifier from the marketplace " +
					"(e.g. `langgenius/openai:0.3.8@<sha256>`).",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plugin_id": schema.StringAttribute{
				MarkdownDescription: "Plugin ID assigned by Dify (the identifier without the version, e.g. `langgenius/openai`).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Installed plugin version.",
				Computed:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Plugin category (e.g. `model`, `tool`).",
				Computed:            true,
			},
		},
	}
}

func (r *PluginInstallResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// findInstalledPlugin locates the installed plugin matching the configured
// identifier: first by exact identifier, then by plugin_id prefix.
func findInstalledPlugin(plugins []PluginInfo, identifier string) *PluginInfo {
	pluginIDPrefix := strings.SplitN(identifier, ":", 2)[0]
	for i := range plugins {
		if plugins[i].Identifier == identifier {
			return &plugins[i]
		}
	}
	for i := range plugins {
		if plugins[i].PluginID == pluginIDPrefix {
			return &plugins[i]
		}
	}
	return nil
}

func (r *PluginInstallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginInstallResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := data.PluginUniqueIdentifier.ValueString()

	if err := r.client.InstallPlugin(ctx, identifier); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to install plugin: %s", err))
		return
	}

	plugins, err := r.client.ListPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list plugins: %s", err))
		return
	}

	installed := findInstalledPlugin(plugins, identifier)
	if installed == nil {
		resp.Diagnostics.AddError("Plugin Not Found", fmt.Sprintf("Plugin '%s' is not in the installed plugins list after install", identifier))
		return
	}

	data.PluginID = types.StringValue(installed.PluginID)
	data.Version = types.StringValue(installed.Version)
	data.Category = types.StringValue(installed.Category)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginInstallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PluginInstallResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plugins, err := r.client.ListPlugins(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list plugins: %s", err))
		return
	}

	pluginID := data.PluginID.ValueString()
	var installed *PluginInfo
	for i := range plugins {
		if plugins[i].PluginID == pluginID {
			installed = &plugins[i]
			break
		}
	}

	if installed == nil {
		// Plugin no longer installed — remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// The identifier records which exact version was installed; if the server
	// reports a different one the resource was replaced out of band — record it
	// so the next plan requires replacement.
	if installed.Identifier != data.PluginUniqueIdentifier.ValueString() {
		data.PluginUniqueIdentifier = types.StringValue(installed.Identifier)
	}
	data.Version = types.StringValue(installed.Version)
	data.Category = types.StringValue(installed.Category)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginInstallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Plugin install is replace-only (plugin_unique_identifier uses RequiresReplace).
	// Update should never be called, but handle it gracefully.
	var data PluginInstallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginInstallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PluginInstallResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UninstallPlugin(ctx, data.PluginID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to uninstall plugin: %s", err))
		return
	}
}
