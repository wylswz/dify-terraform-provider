// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure DifyProvider satisfies various provider interfaces.
var _ provider.Provider = &DifyProvider{}

// DifyProvider defines the provider implementation.
type DifyProvider struct {
	version string
}

// DifyProviderModel describes the provider data model.
type DifyProviderModel struct {
	Host        types.String `tfsdk:"host"`
	APIKey      types.String `tfsdk:"api_key"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (p *DifyProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "dify"
	resp.Version = p.version
}

func (p *DifyProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Dify provider allows managing Dify applications, model provider credentials, " +
			"plugins, and API keys as infrastructure-as-code via Terraform.",

		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Dify API base URL (e.g. `https://dify.example.com`). " +
					"Can also be set via `DIFY_HOST` environment variable.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Admin API key for authentication (X-Admin-Api-Key header). " +
					"Can also be set via `DIFY_API_KEY` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"workspace_id": schema.StringAttribute{
				MarkdownDescription: "Default workspace ID for resource operations. " +
					"Can also be set via `DIFY_WORKSPACE_ID` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *DifyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data DifyProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values from environment variables
	host := data.Host.ValueString()
	if host == "" {
		host = envOr("DIFY_HOST", "http://localhost")
	}

	apiKey := data.APIKey.ValueString()
	if apiKey == "" {
		apiKey = envOr("DIFY_API_KEY", "")
	}

	workspaceID := data.WorkspaceID.ValueString()
	if workspaceID == "" {
		workspaceID = envOr("DIFY_WORKSPACE_ID", "")
	}

	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Dify API Key",
			"api_key must be provided either in the provider config or via the DIFY_API_KEY environment variable.",
		)
		return
	}

	if workspaceID == "" {
		resp.Diagnostics.AddError(
			"Missing Dify Workspace ID",
			"workspace_id must be provided either in the provider config or via the DIFY_WORKSPACE_ID environment variable.",
		)
		return
	}

	client := NewDifyClient(host, apiKey, workspaceID)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *DifyProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAppResource,
		NewModelProviderCredentialResource,
		NewToolProviderCredentialResource,
		NewPluginInstallResource,
		NewAppAPIKeyResource,
		NewDatasetResource,
		NewDatasetDocumentResource,
	}
}

func (p *DifyProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAppDataSource,
		NewModelProvidersDataSource,
		NewPluginsDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DifyProvider{
			version: version,
		}
	}
}

// envOr returns the environment variable value or the fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
