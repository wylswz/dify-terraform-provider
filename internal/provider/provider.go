// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
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
	Token       types.String `tfsdk:"token"`
	WorkspaceID types.String `tfsdk:"workspace_id"`
}

func (p *DifyProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "dify"
	resp.Version = p.version
}

func (p *DifyProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Dify provider manages Dify applications, model and tool provider " +
			"credentials, plugin installs, and app API keys as infrastructure-as-code via Terraform. " +
			"It authenticates with a Personal Access Token against the workspace-scoped OpenAPI.",

		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Dify base URL (e.g. `https://dify.example.com`). The provider calls " +
					"`<host>/openapi/v1`. Can also be set via `DIFY_HOST` environment variable.",
				Optional: true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Personal Access Token for authentication (`Authorization: Bearer` header). " +
					"Create one in the Dify console under account settings → Personal Access Tokens; " +
					"it must be scoped to the workspace. Can also be set via `DIFY_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"workspace_id": schema.StringAttribute{
				MarkdownDescription: "Default workspace ID for resource operations. " +
					"Can also be set via `DIFY_WORKSPACE_ID` environment variable. " +
					"When neither is set, the provider resolves the token owner's default " +
					"workspace from `GET /openapi/v1/account`.",
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

	token := data.Token.ValueString()
	if token == "" {
		token = envOr("DIFY_TOKEN", "")
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Dify Personal Access Token",
			"token must be provided either in the provider config or via the DIFY_TOKEN environment variable.",
		)
		return
	}

	client := NewDifyClient(host, token, "")

	workspaceID, err := resolveWorkspaceID(ctx, client, data.WorkspaceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Resolve Dify Workspace",
			fmt.Sprintf("workspace_id was not set and the account lookup failed: %s", err),
		)
		return
	}
	if workspaceID == "" {
		resp.Diagnostics.AddError(
			"Missing Dify Workspace ID",
			"No workspace_id in config or DIFY_WORKSPACE_ID, and GET /openapi/v1/account "+
				"reported no default workspace. Set workspace_id in the provider config or "+
				"export DIFY_WORKSPACE_ID.",
		)
		return
	}
	client.WorkspaceID = workspaceID
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *DifyProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAppResource,
		NewModelProviderCredentialResource,
		NewModelCredentialResource,
		NewToolProviderCredentialResource,
		NewPluginInstallResource,
		NewAppAPIKeyResource,
		NewDefaultModelResource,
		NewModelSettingsResource,
		NewAPIToolProviderResource,
		NewMCPProviderResource,
		NewWorkflowToolResource,
	}
}

func (p *DifyProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAppDataSource,
		NewModelsDataSource,
		NewPluginsDataSource,
		NewToolsDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DifyProvider{
			version: version,
		}
	}
}

// resolveWorkspaceID applies the workspace resolution order: the configured
// value, then DIFY_WORKSPACE_ID, then the token owner's default workspace
// from GET /openapi/v1/account.
func resolveWorkspaceID(ctx context.Context, client *DifyClient, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if env := envOr("DIFY_WORKSPACE_ID", ""); env != "" {
		return env, nil
	}
	account, err := client.GetAccount(ctx)
	if err != nil {
		return "", err
	}
	return account.DefaultWorkspaceID, nil
}

// envOr returns the environment variable value or the fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
