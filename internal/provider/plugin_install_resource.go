// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

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
	PluginInstallationID   types.String `tfsdk:"plugin_installation_id"`
	Source                 types.String `tfsdk:"source"`
	Repo                   types.String `tfsdk:"repo"`
	Version                types.String `tfsdk:"version"`
	Package                types.String `tfsdk:"package"`
}

func (r *PluginInstallResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_install"
}

func (r *PluginInstallResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a plugin installation in Dify. Supports installing from marketplace or GitHub.",

		Attributes: map[string]schema.Attribute{
			"plugin_unique_identifier": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for the plugin (e.g. `langgenius/ollama:0.0.1`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plugin_installation_id": schema.StringAttribute{
				MarkdownDescription: "Installation ID (assigned by Dify after install).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Installation source: `marketplace` (default) or `github`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repo": schema.StringAttribute{
				MarkdownDescription: "GitHub repository (e.g. `langgenius/dify-plugin-ollama`). Only for `github` source.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Version tag or branch. Only for `github` source.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package": schema.StringAttribute{
				MarkdownDescription: "Package path within the repo. Only for `github` source.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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

func (r *PluginInstallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PluginInstallResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	source := data.Source.ValueString()
	if source == "" {
		source = "marketplace"
		data.Source = types.StringValue("marketplace")
	}

	switch source {
	case "github":
		githubReq := PluginInstallGithubRequest{
			PluginUniqueIdentifier: data.PluginUniqueIdentifier.ValueString(),
			Repo:                   data.Repo.ValueString(),
			Version:                data.Version.ValueString(),
			Package:                data.Package.ValueString(),
		}
		err := r.client.InstallPluginFromGithub(ctx, githubReq)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to install plugin from GitHub: %s", err))
			return
		}
	case "marketplace":
		marketplaceReq := PluginInstallMarketplaceRequest{
			PluginUniqueIdentifiers: []string{data.PluginUniqueIdentifier.ValueString()},
		}
		err := r.client.InstallPluginsFromMarketplace(ctx, marketplaceReq)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to install plugin from marketplace: %s", err))
			return
		}
	default:
		resp.Diagnostics.AddError("Invalid Source", fmt.Sprintf("source must be 'marketplace' or 'github', got: %s", source))
		return
	}

	// Poll until the plugin appears in the installed list (install is async)
	identifier := data.PluginUniqueIdentifier.ValueString()
	var installationID string
	found := false

	for range 180 {
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Context Cancelled", "Plugin install polling cancelled")
			return
		case <-time.After(2 * time.Second):
		}

		plugins, err := r.client.ListPlugins(ctx)
		if err != nil {
			continue // retry on transient errors
		}

		for _, p := range plugins.Plugins {
			if p.PluginUniqueIdentifier == identifier {
				installationID = p.PluginInstallationID
				found = true
				break
			}
		}

		if found {
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError("Plugin Not Ready", fmt.Sprintf("Plugin '%s' not found in installed list after 360s of polling", identifier))
		return
	}

	data.PluginInstallationID = types.StringValue(installationID)

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

	found := false
	identifier := data.PluginUniqueIdentifier.ValueString()
	for _, p := range plugins.Plugins {
		if p.PluginUniqueIdentifier == identifier {
			data.PluginInstallationID = types.StringValue(p.PluginInstallationID)
			found = true
			break
		}
	}

	if !found {
		// Plugin no longer installed — remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PluginInstallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Plugin install is replace-only (all fields use RequiresReplace).
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

	uninstallReq := PluginUninstallRequest{
		PluginInstallationID: data.PluginInstallationID.ValueString(),
	}

	err := r.client.UninstallPlugin(ctx, uninstallReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to uninstall plugin: %s", err))
		return
	}
}
