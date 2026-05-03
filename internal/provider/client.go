// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DifyClient is a thin HTTP client for the Dify admin API.
type DifyClient struct {
	BaseURL     string
	APIKey      string
	WorkspaceID string
	HTTPClient  *http.Client
}

// NewDifyClient creates a new DifyClient from provider configuration.
func NewDifyClient(host, apiKey, workspaceID string) *DifyClient {
	baseURL := strings.TrimRight(host, "/")
	return &DifyClient{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		WorkspaceID: workspaceID,
		HTTPClient:  &http.Client{},
	}
}

// provisioningURL builds the full URL for a provisioning endpoint.
func (c *DifyClient) provisioningURL(path string) string {
	return fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s%s", c.BaseURL, c.WorkspaceID, path)
}

// doRequest performs an HTTP request with the admin API key header.
func (c *DifyClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Api-Key", c.APIKey)

	return c.HTTPClient.Do(req)
}

// APIError represents an HTTP error response from the Dify API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Body)
}

// readResponseBody reads and unmarshals the response body.
func readResponseBody(resp *http.Response, target any) error {
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}

	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(data))
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// App API
// ---------------------------------------------------------------------------

// AppCreateRequest is the payload for creating an app.
type AppCreateRequest struct {
	YAMLContent  string `json:"yaml_content"`
	CreatorEmail string `json:"creator_email"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
}

// AppUpdateRequest is the payload for updating an app.
type AppUpdateRequest struct {
	YAMLContent string `json:"yaml_content"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// AppResponse is the response from the app API.
type AppResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	Description    string `json:"description"`
	IconType       string `json:"icon_type"`
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
	EnableSite     bool   `json:"enable_site"`
	EnableAPI      bool   `json:"enable_api"`
	DSLYaml        string `json:"dsl_yaml"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ImportResponse is the response from the DSL import API.
type ImportResponse struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	AppID              string `json:"app_id"`
	AppMode            string `json:"app_mode"`
	CurrentDSLVersion  string `json:"current_dsl_version"`
	ImportedDSLVersion string `json:"imported_dsl_version"`
	Error              string `json:"error"`
}

// AppListItem is a single app in the list response.
type AppListItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	Description    string `json:"description"`
	IconType       string `json:"icon_type"`
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
	EnableSite     bool   `json:"enable_site"`
	EnableAPI      bool   `json:"enable_api"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// AppListResponse is the response from the app list API.
type AppListResponse struct {
	Data []AppListItem `json:"data"`
}

// CreateApp creates a new app via DSL import.
func (c *DifyClient) CreateApp(ctx context.Context, req AppCreateRequest) (*ImportResponse, error) {
	url := c.provisioningURL("/apps")
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return nil, err
	}

	var result ImportResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	if result.Status == "failed" {
		return nil, fmt.Errorf("import failed: %s", result.Error)
	}

	return &result, nil
}

// GetApp retrieves an app with its DSL export.
func (c *DifyClient) GetApp(ctx context.Context, appID string) (*AppResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s", c.BaseURL, c.WorkspaceID, appID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result AppResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateApp updates an app by re-importing DSL.
func (c *DifyClient) UpdateApp(ctx context.Context, appID string, req AppUpdateRequest) (*ImportResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s", c.BaseURL, c.WorkspaceID, appID)
	resp, err := c.doRequest(ctx, http.MethodPut, url, req)
	if err != nil {
		return nil, err
	}

	var result ImportResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	if result.Status == "failed" {
		return nil, fmt.Errorf("import failed: %s", result.Error)
	}

	return &result, nil
}

// DeleteApp deletes an app.
func (c *DifyClient) DeleteApp(ctx context.Context, appID string) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s", c.BaseURL, c.WorkspaceID, appID)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ListApps lists apps in the workspace.
func (c *DifyClient) ListApps(ctx context.Context) (*AppListResponse, error) {
	url := c.provisioningURL("/apps")
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result AppListResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// Model Provider Credential API
// ---------------------------------------------------------------------------

// CredentialCreateRequest is the payload for creating a credential.
type CredentialCreateRequest struct {
	Credentials map[string]any `json:"credentials"`
	Name        string         `json:"name,omitempty"`
}

// CredentialUpdateRequest is the payload for updating a credential.
type CredentialUpdateRequest struct {
	CredentialID string         `json:"credential_id"`
	Credentials  map[string]any `json:"credentials"`
	Name         string         `json:"name,omitempty"`
}

// CredentialDeleteRequest is the payload for deleting a credential.
type CredentialDeleteRequest struct {
	CredentialID string `json:"credential_id"`
}

// CredentialSwitchRequest is the payload for switching active credential.
type CredentialSwitchRequest struct {
	CredentialID string `json:"credential_id"`
}

// CredentialResponse is the response from the credential API.
type CredentialResponse struct {
	Credentials  map[string]any `json:"credentials"`
	CredentialID string         `json:"credential_id"`
}

// CredentialCreateResponse is the response from creating a credential.
type CredentialCreateResponse struct {
	Result       string `json:"result"`
	CredentialID string `json:"credential_id"`
}

// ModelProvider represents a model provider.
type ModelProvider struct {
	Provider string `json:"provider"`
}

// ModelProviderListResponse is the response from listing model providers.
type ModelProviderListResponse struct {
	Data []ModelProvider `json:"data"`
}

// GetCredential retrieves credentials for a model provider.
func (c *DifyClient) GetCredential(ctx context.Context, provider string) (*CredentialResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result CredentialResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateCredential creates a new credential for a model provider.
func (c *DifyClient) CreateCredential(ctx context.Context, provider string, req CredentialCreateRequest) (*CredentialCreateResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return nil, err
	}

	var result CredentialCreateResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateCredential updates an existing credential for a model provider.
func (c *DifyClient) UpdateCredential(ctx context.Context, provider string, req CredentialUpdateRequest) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodPut, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// DeleteCredential deletes a credential for a model provider.
func (c *DifyClient) DeleteCredential(ctx context.Context, provider string, req CredentialDeleteRequest) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// SwitchCredential switches the active credential for a model provider.
func (c *DifyClient) SwitchCredential(ctx context.Context, provider string, req CredentialSwitchRequest) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers/%s/credentials/switch", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ListModelProviders lists model providers in the workspace.
func (c *DifyClient) ListModelProviders(ctx context.Context) (*ModelProviderListResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/model-providers", c.BaseURL, c.WorkspaceID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result ModelProviderListResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// Tool Provider Credential API
// ---------------------------------------------------------------------------

// GetToolCredential retrieves credentials for a builtin tool provider.
func (c *DifyClient) GetToolCredential(ctx context.Context, provider string) (*CredentialResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/tool-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result CredentialResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateToolCredential creates a new credential for a builtin tool provider.
func (c *DifyClient) CreateToolCredential(ctx context.Context, provider string, req CredentialCreateRequest) (*CredentialCreateResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/tool-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return nil, err
	}

	var result CredentialCreateResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateToolCredential updates an existing credential for a builtin tool provider.
func (c *DifyClient) UpdateToolCredential(ctx context.Context, provider string, req CredentialUpdateRequest) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/tool-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodPut, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// DeleteToolCredential deletes a credential for a builtin tool provider.
func (c *DifyClient) DeleteToolCredential(ctx context.Context, provider string, req CredentialDeleteRequest) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/tool-providers/%s/credentials", c.BaseURL, c.WorkspaceID, provider)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ---------------------------------------------------------------------------
// Plugin API
// ---------------------------------------------------------------------------

// PluginInstallMarketplaceRequest is the payload for installing plugins from marketplace.
type PluginInstallMarketplaceRequest struct {
	PluginUniqueIdentifiers []string `json:"plugin_unique_identifiers"`
}

// PluginInstallGithubRequest is the payload for installing a plugin from GitHub.
type PluginInstallGithubRequest struct {
	PluginUniqueIdentifier string `json:"plugin_unique_identifier"`
	Repo                   string `json:"repo"`
	Version                string `json:"version"`
	Package                string `json:"package"`
}

// PluginUninstallRequest is the payload for uninstalling a plugin.
type PluginUninstallRequest struct {
	PluginInstallationID string `json:"plugin_installation_id"`
}

// PluginItem represents an installed plugin.
type PluginItem struct {
	PluginID               string `json:"plugin_id"`
	PluginUniqueIdentifier string `json:"plugin_unique_identifier"`
	PluginInstallationID   string `json:"plugin_installation_id"`
	Name                   string `json:"name"`
}

// PluginListResponse is the response from listing plugins.
type PluginListResponse struct {
	Plugins []PluginItem `json:"plugins"`
	Total   int          `json:"total"`
}

// InstallPluginsFromMarketplace installs plugins from marketplace.
func (c *DifyClient) InstallPluginsFromMarketplace(ctx context.Context, req PluginInstallMarketplaceRequest) error {
	url := c.provisioningURL("/plugins/install/marketplace")
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// InstallPluginFromGithub installs a plugin from GitHub.
func (c *DifyClient) InstallPluginFromGithub(ctx context.Context, req PluginInstallGithubRequest) error {
	url := c.provisioningURL("/plugins/install/github")
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// UninstallPlugin uninstalls a plugin.
func (c *DifyClient) UninstallPlugin(ctx context.Context, req PluginUninstallRequest) error {
	url := c.provisioningURL("/plugins/uninstall")
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ListPlugins lists installed plugins in the workspace.
func (c *DifyClient) ListPlugins(ctx context.Context) (*PluginListResponse, error) {
	url := c.provisioningURL("/plugins")
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result PluginListResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// App API Key API
// ---------------------------------------------------------------------------

// APIKeyItem represents an API key.
type APIKeyItem struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	CreatedAt string `json:"created_at"`
}

// APIKeyListResponse is the response from listing API keys.
type APIKeyListResponse struct {
	Data []APIKeyItem `json:"data"`
}

// CreateAppAPIKey creates a new API key for an app.
func (c *DifyClient) CreateAppAPIKey(ctx context.Context, appID string) (*APIKeyItem, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s/api-keys", c.BaseURL, c.WorkspaceID, appID)
	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	var result APIKeyItem
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListAppAPIKeys lists API keys for an app.
func (c *DifyClient) ListAppAPIKeys(ctx context.Context, appID string) (*APIKeyListResponse, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s/api-keys", c.BaseURL, c.WorkspaceID, appID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result APIKeyListResponse
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteAppAPIKey deletes an API key for an app.
func (c *DifyClient) DeleteAppAPIKey(ctx context.Context, appID, apiKeyID string) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/apps/%s/api-keys/%s", c.BaseURL, c.WorkspaceID, appID, apiKeyID)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ---------------------------------------------------------------------------
// Dataset API
// ---------------------------------------------------------------------------

// Dataset represents a Dify dataset.
type Dataset struct {
	ID                     string         `json:"id"`
	Name                   string         `json:"name"`
	Description            string         `json:"description"`
	IndexingTechnique      string         `json:"indexing_technique"`
	Permission             string         `json:"permission"`
	ProcessRule            map[string]any `json:"process_rule"`
	EmbeddingModel         string         `json:"embedding_model"`
	EmbeddingModelProvider string         `json:"embedding_model_provider"`
}

// DatasetCreateRequest is the payload for creating a dataset.
type DatasetCreateRequest struct {
	Name                   string         `json:"name"`
	Description            string         `json:"description"`
	IndexingTechnique      string         `json:"indexing_technique,omitempty"`
	Permission             string         `json:"permission,omitempty"`
	ProcessRule            map[string]any `json:"process_rule,omitempty"`
	EmbeddingModel         string         `json:"embedding_model,omitempty"`
	EmbeddingModelProvider string         `json:"embedding_model_provider,omitempty"`
	CreatorEmail           string         `json:"creator_email"`
}

// DatasetUpdateRequest is the payload for updating a dataset.
type DatasetUpdateRequest struct {
	Name                   string         `json:"name,omitempty"`
	Description            string         `json:"description,omitempty"`
	IndexingTechnique      string         `json:"indexing_technique,omitempty"`
	Permission             string         `json:"permission,omitempty"`
	ProcessRule            map[string]any `json:"process_rule,omitempty"`
	EmbeddingModel         string         `json:"embedding_model,omitempty"`
	EmbeddingModelProvider string         `json:"embedding_model_provider,omitempty"`
}

// ListDatasets lists all datasets in the workspace.
func (c *DifyClient) ListDatasets(ctx context.Context) ([]Dataset, error) {
	url := c.provisioningURL("/datasets")
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result []Dataset
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetDataset retrieves a dataset by ID.
func (c *DifyClient) GetDataset(ctx context.Context, datasetID string) (*Dataset, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s", c.BaseURL, c.WorkspaceID, datasetID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result Dataset
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateDataset creates a new dataset.
func (c *DifyClient) CreateDataset(ctx context.Context, req DatasetCreateRequest) (*Dataset, error) {
	url := c.provisioningURL("/datasets")
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return nil, err
	}

	var result Dataset
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateDataset updates a dataset.
func (c *DifyClient) UpdateDataset(ctx context.Context, datasetID string, req DatasetUpdateRequest) (*Dataset, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s", c.BaseURL, c.WorkspaceID, datasetID)
	resp, err := c.doRequest(ctx, http.MethodPatch, url, req)
	if err != nil {
		return nil, err
	}

	var result Dataset
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteDataset deletes a dataset.
func (c *DifyClient) DeleteDataset(ctx context.Context, datasetID string) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s", c.BaseURL, c.WorkspaceID, datasetID)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}

// ---------------------------------------------------------------------------
// Dataset Document API
// ---------------------------------------------------------------------------

// DatasetDocument represents a document in a dataset.
type DatasetDocument struct {
	ID                     string `json:"id"`
	DatasetID              string `json:"dataset_id"`
	Name                   string `json:"name"`
	DataSourceType         string `json:"data_source_type"`
	IndexingStatus         string `json:"indexing_status"`
	IndexingTechnique      string `json:"indexing_technique"`
	EmbeddingModel         string `json:"embedding_model"`
	EmbeddingModelProvider string `json:"embedding_model_provider"`
}

// DatasetDocumentCreateRequest is the payload for creating a document.
type DatasetDocumentCreateRequest struct {
	DataSourceType         string         `json:"data_source_type"`
	DataSourceInfo         map[string]any `json:"data_source_info"`
	IndexingTechnique      string         `json:"indexing_technique,omitempty"`
	EmbeddingModel         string         `json:"embedding_model,omitempty"`
	EmbeddingModelProvider string         `json:"embedding_model_provider,omitempty"`
	CreatorEmail           string         `json:"creator_email"`
}

// GetDatasetDocument retrieves a document by ID.
func (c *DifyClient) GetDatasetDocument(ctx context.Context, datasetID, documentID string) (*DatasetDocument, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s/documents/%s", c.BaseURL, c.WorkspaceID, datasetID, documentID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result DatasetDocument
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreateDatasetDocument creates a new document in a dataset.
func (c *DifyClient) CreateDatasetDocument(ctx context.Context, datasetID string, req DatasetDocumentCreateRequest) (*DatasetDocument, error) {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s/documents", c.BaseURL, c.WorkspaceID, datasetID)
	resp, err := c.doRequest(ctx, http.MethodPost, url, req)
	if err != nil {
		return nil, err
	}

	var result DatasetDocument
	if err := readResponseBody(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteDatasetDocument deletes a document.
func (c *DifyClient) DeleteDatasetDocument(ctx context.Context, datasetID, documentID string) error {
	url := fmt.Sprintf("%s/admin/api/provisioning/workspaces/%s/datasets/%s/documents/%s", c.BaseURL, c.WorkspaceID, datasetID, documentID)
	resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	return readResponseBody(resp, nil)
}
