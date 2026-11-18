// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------------------
// API tool providers (workspaces/{ws}/api-tool-providers)
// ---------------------------------------------------------------------------

// APIToolProvider is an OpenAPI-schema-backed tool provider, keyed by name.
// Credentials come back masked; secret values are write-only.
type APIToolProvider struct {
	Name             string            `json:"name"`
	Icon             map[string]string `json:"icon"`
	SchemaType       string            `json:"schema_type"`
	Schema           string            `json:"schema"`
	Credentials      map[string]any    `json:"credentials"`
	PrivacyPolicy    *string           `json:"privacy_policy"`
	CustomDisclaimer *string           `json:"custom_disclaimer"`
	Labels           []string          `json:"labels"`
	Tools            []string          `json:"tools"`
	Description      *string           `json:"description"`
}

type apiToolProviderRequest struct {
	Name             string            `json:"name"`
	Icon             map[string]string `json:"icon"`
	Credentials      map[string]any    `json:"credentials"`
	SchemaType       string            `json:"schema_type"`
	Schema           string            `json:"schema"`
	PrivacyPolicy    *string           `json:"privacy_policy,omitempty"`
	CustomDisclaimer *string           `json:"custom_disclaimer,omitempty"`
	Labels           []string          `json:"labels,omitempty"`
}

// CreateAPIToolProvider adds an API tool provider (201 with the describe payload).
func (c *DifyClient) CreateAPIToolProvider(ctx context.Context, req apiToolProviderRequest) (*APIToolProvider, error) {
	var result APIToolProvider
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/api-tool-providers"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAPIToolProvider reads one API tool provider by name.
func (c *DifyClient) GetAPIToolProvider(ctx context.Context, name string) (*APIToolProvider, error) {
	var result APIToolProvider
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/api-tool-providers/"+url.PathEscape(name)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateAPIToolProvider replaces an API tool provider. The request's Name may
// differ from providerName — that renames the provider.
func (c *DifyClient) UpdateAPIToolProvider(ctx context.Context, providerName string, req apiToolProviderRequest) (*APIToolProvider, error) {
	var result APIToolProvider
	if err := c.doJSON(ctx, http.MethodPut, c.workspaceURL("/api-tool-providers/"+url.PathEscape(providerName)), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteAPIToolProvider deletes an API tool provider by name.
func (c *DifyClient) DeleteAPIToolProvider(ctx context.Context, name string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspaceURL("/api-tool-providers/"+url.PathEscape(name)), nil, nil)
}

// ---------------------------------------------------------------------------
// MCP providers (workspaces/{ws}/mcp-providers)
// ---------------------------------------------------------------------------

// MCPConfiguration carries the connection settings of an MCP provider.
type MCPConfiguration struct {
	Timeout        float64 `json:"timeout"`
	SSEReadTimeout float64 `json:"sse_read_timeout"`
}

// MCPProvider is a remote MCP server registered in the workspace. Secret
// fields (server_url, masked_headers, authentication) come back masked.
type MCPProvider struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Icon                  json.RawMessage   `json:"icon"`
	ServerURL             *string           `json:"server_url"`
	ServerIdentifier      *string           `json:"server_identifier"`
	MaskedHeaders         map[string]string `json:"masked_headers"`
	Authentication        json.RawMessage   `json:"authentication"`
	IsTeamAuthorization   bool              `json:"is_team_authorization"`
	IsDynamicRegistration bool              `json:"is_dynamic_registration"`
	Configuration         *MCPConfiguration `json:"configuration"`
	IdentityMode          string            `json:"identity_mode"`
	UpdatedAt             int64             `json:"updated_at"`
	Tools                 []string          `json:"tools"`
}

type mcpProviderRequest struct {
	ServerURL        string            `json:"server_url"`
	Name             string            `json:"name"`
	Icon             string            `json:"icon"`
	IconType         string            `json:"icon_type"`
	IconBackground   string            `json:"icon_background,omitempty"`
	ServerIdentifier string            `json:"server_identifier"`
	Configuration    *MCPConfiguration `json:"configuration,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}

// CreateMCPProvider registers an MCP provider (201). The server stores it with
// identity forwarding off and makes a best-effort connection to fetch tools.
func (c *DifyClient) CreateMCPProvider(ctx context.Context, req mcpProviderRequest) (*MCPProvider, error) {
	var result MCPProvider
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/mcp-providers"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMCPProvider reads one MCP provider by id.
func (c *DifyClient) GetMCPProvider(ctx context.Context, providerID string) (*MCPProvider, error) {
	var result MCPProvider
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/mcp-providers/"+url.PathEscape(providerID)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateMCPProvider replaces an MCP provider; it keeps its identity mode.
func (c *DifyClient) UpdateMCPProvider(ctx context.Context, providerID string, req mcpProviderRequest) (*MCPProvider, error) {
	var result MCPProvider
	if err := c.doJSON(ctx, http.MethodPut, c.workspaceURL("/mcp-providers/"+url.PathEscape(providerID)), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteMCPProvider deletes an MCP provider by id.
func (c *DifyClient) DeleteMCPProvider(ctx context.Context, providerID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspaceURL("/mcp-providers/"+url.PathEscape(providerID)), nil, nil)
}

// RefreshMCPProvider re-fetches the provider's tools from its server.
func (c *DifyClient) RefreshMCPProvider(ctx context.Context, providerID string) (*MCPProvider, error) {
	var result MCPProvider
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/mcp-providers/"+url.PathEscape(providerID)+":refresh"), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Workflow tools (workspaces/{ws}/workflow-tools)
// ---------------------------------------------------------------------------

// WorkflowToolParameter is one parameter of a workflow tool.
type WorkflowToolParameter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Form        string `json:"form"`
}

// WorkflowTool is a workflow app published as a tool provider.
type WorkflowTool struct {
	Name           string                  `json:"name"`
	Label          string                  `json:"label"`
	WorkflowToolID string                  `json:"workflow_tool_id"`
	WorkflowAppID  string                  `json:"workflow_app_id"`
	Icon           map[string]string       `json:"icon"`
	Description    string                  `json:"description"`
	Parameters     []WorkflowToolParameter `json:"parameters"`
	OutputSchema   map[string]any          `json:"output_schema"`
	Synced         bool                    `json:"synced"`
	PrivacyPolicy  *string                 `json:"privacy_policy"`
}

type workflowToolRequest struct {
	WorkflowAppID string                  `json:"workflow_app_id,omitempty"`
	Name          string                  `json:"name"`
	Label         string                  `json:"label"`
	Icon          map[string]string       `json:"icon"`
	Description   string                  `json:"description"`
	Parameters    []WorkflowToolParameter `json:"parameters"`
	PrivacyPolicy *string                 `json:"privacy_policy,omitempty"`
	Labels        []string                `json:"labels,omitempty"`
}

// CreateWorkflowTool publishes a workflow app as a tool (201). The app must
// have a published workflow — see dify_app.publish.
func (c *DifyClient) CreateWorkflowTool(ctx context.Context, req workflowToolRequest) (*WorkflowTool, error) {
	var result WorkflowTool
	if err := c.doJSON(ctx, http.MethodPost, c.workspaceURL("/workflow-tools"), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetWorkflowTool reads one workflow tool by id.
func (c *DifyClient) GetWorkflowTool(ctx context.Context, workflowToolID string) (*WorkflowTool, error) {
	var result WorkflowTool
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/workflow-tools/"+url.PathEscape(workflowToolID)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateWorkflowTool replaces a workflow tool; it keeps backing the same app.
func (c *DifyClient) UpdateWorkflowTool(ctx context.Context, workflowToolID string, req workflowToolRequest) (*WorkflowTool, error) {
	var result WorkflowTool
	if err := c.doJSON(ctx, http.MethodPut, c.workspaceURL("/workflow-tools/"+url.PathEscape(workflowToolID)), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteWorkflowTool unpublishes a workflow tool by id.
func (c *DifyClient) DeleteWorkflowTool(ctx context.Context, workflowToolID string) error {
	return c.doJSON(ctx, http.MethodDelete, c.workspaceURL("/workflow-tools/"+url.PathEscape(workflowToolID)), nil, nil)
}

// ---------------------------------------------------------------------------
// Tools API (data source)
// ---------------------------------------------------------------------------

// ToolInfo is one row of GET /workspaces/{ws}/tools. NodeData carries the
// server-provided workflow tool-node template (provider_id, provider_name,
// plugin_unique_identifier, tool_parameters, ...) verbatim.
type ToolInfo struct {
	Provider      string          `json:"provider"`
	ProviderType  string          `json:"provider_type"`
	ProviderLabel *string         `json:"provider_label"`
	Configured    bool            `json:"configured"`
	Name          string          `json:"name"`
	Label         string          `json:"label"`
	Description   *string         `json:"description"`
	NodeData      json.RawMessage `json:"node_data"`
}

type toolListResponse struct {
	HasMore bool       `json:"has_more"`
	Data    []ToolInfo `json:"data"`
}

// ListTools returns every tool in the workspace, following pagination.
// provider and query are optional filters; empty means no filter.
func (c *DifyClient) ListTools(ctx context.Context, provider, query string) ([]ToolInfo, error) {
	var all []ToolInfo
	for page := 1; ; page++ {
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("limit", "100")
		if provider != "" {
			values.Set("provider", provider)
		}
		if query != "" {
			values.Set("query", query)
		}

		url := c.workspaceURL("/tools") + "?" + values.Encode()
		var result toolListResponse
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
			return nil, err
		}

		all = append(all, result.Data...)
		if !result.HasMore {
			return all, nil
		}
	}
}
