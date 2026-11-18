// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DifyClient is a thin HTTP client for the Dify user-scoped OpenAPI
// (`/openapi/v1`), authenticated with a Personal Access Token.
type DifyClient struct {
	BaseURL     string
	Token       string
	WorkspaceID string
	HTTPClient  *http.Client

	// pluginPollInterval and pluginPollTimeout control how InstallPlugin waits
	// for an asynchronous plugin install task. They are fields so tests can
	// shrink the interval.
	pluginPollInterval time.Duration
	pluginPollTimeout  time.Duration

	// catalog is the X-Dify-Catalog fingerprint guarding /openapi/v1 requests.
	// It is refreshed from response headers and from GET /_version.
	mu      sync.Mutex
	catalog string

	// sleep waits between rate-limit retries; a field so tests can stub it.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewDifyClient creates a new DifyClient from provider configuration.
func NewDifyClient(host, token, workspaceID string) *DifyClient {
	baseURL := strings.TrimRight(host, "/") + "/openapi/v1"
	return &DifyClient{
		BaseURL:            baseURL,
		Token:              token,
		WorkspaceID:        workspaceID,
		HTTPClient:         &http.Client{},
		pluginPollInterval: 3 * time.Second,
		pluginPollTimeout:  10 * time.Minute,
		sleep:              sleepCtx,
	}
}

// workspaceURL builds the full URL for a workspace-scoped endpoint.
func (c *DifyClient) workspaceURL(path string) string {
	return fmt.Sprintf("%s/workspaces/%s%s", c.BaseURL, url.PathEscape(c.WorkspaceID), path)
}

// doRequest performs an HTTP request with the bearer token and catalog
// fingerprint headers. Guarded endpoints reject stale fingerprints with
// 412 {"code":"catalog_stale"} (retried once) and rate-limit with
// 429 {"code":"too_many_requests"} plus a Retry-After header (retried up to
// maxRateLimitRetries times, honouring ctx cancellation while waiting).
func (c *DifyClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var bodyBytes []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyBytes = data
	}

	if err := c.ensureCatalog(ctx); err != nil {
		return nil, err
	}

	const maxRateLimitRetries = 5

	catalogRetried := false
	rateLimitRetries := 0
	backoff := time.Second

	for {
		var reqBody io.Reader
		if len(bodyBytes) > 0 {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if len(bodyBytes) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Dify-Catalog", c.getCatalog())

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		c.rememberCatalog(resp)

		if resp.StatusCode == http.StatusPreconditionFailed && !catalogRetried {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("failed to read response body: %w", readErr)
			}
			if isCatalogStale(data) {
				catalogRetried = true
				// rememberCatalog already stored the response's fingerprint;
				// refetch via /_version only when the header was absent.
				if resp.Header.Get("X-Dify-Catalog") == "" {
					if fp, fetchErr := c.fetchCatalog(ctx); fetchErr == nil {
						c.setCatalog(fp)
					}
				}
				continue
			}
			resp.Body = io.NopCloser(bytes.NewReader(data))
			return resp, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests && rateLimitRetries < maxRateLimitRetries {
			rateLimitRetries++
			wait := rateLimitWait(resp, &backoff)
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}
}

// rateLimitWait returns how long to wait before retrying a 429 response: the
// Retry-After header value in whole seconds when present and valid, otherwise
// exponential backoff (advanced through backoff, capped at 30s).
func rateLimitWait(resp *http.Response, backoff *time.Duration) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	wait := *backoff
	*backoff *= 2
	if *backoff > 30*time.Second {
		*backoff = 30 * time.Second
	}
	return wait
}

// sleepCtx waits for d or until ctx is done, returning the ctx error.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// getCatalog returns the stored catalog fingerprint.
func (c *DifyClient) getCatalog() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.catalog
}

// setCatalog stores a catalog fingerprint.
func (c *DifyClient) setCatalog(fingerprint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.catalog = fingerprint
}

// rememberCatalog stores the fingerprint carried by a response's
// X-Dify-Catalog header, when present.
func (c *DifyClient) rememberCatalog(resp *http.Response) {
	if fp := resp.Header.Get("X-Dify-Catalog"); fp != "" {
		c.setCatalog(fp)
	}
}

// ensureCatalog fetches the catalog fingerprint from /_version when none is
// stored yet.
func (c *DifyClient) ensureCatalog(ctx context.Context) error {
	if c.getCatalog() != "" {
		return nil
	}
	fingerprint, err := c.fetchCatalog(ctx)
	if err != nil {
		return err
	}
	c.setCatalog(fingerprint)
	return nil
}

// fetchCatalog retrieves the current catalog fingerprint via an
// unauthenticated GET {BaseURL}/_version.
func (c *DifyClient) fetchCatalog(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/_version", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create catalog request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch catalog fingerprint: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	fingerprint := resp.Header.Get("X-Dify-Catalog")
	if fingerprint == "" {
		return "", fmt.Errorf("catalog fingerprint missing from /_version response (HTTP %d)", resp.StatusCode)
	}
	return fingerprint, nil
}

// isCatalogStale reports whether a response body is the 412 catalog_stale
// error shape.
func isCatalogStale(data []byte) bool {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return false
	}
	return body.Code == "catalog_stale"
}

// APIError represents an HTTP error response from the Dify API.
// The API answers non-2xx with {"code", "message", "status", "hint"?, "details"?};
// Code and Message are empty when the body is not that shape.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Body
	}
	if e.Code != "" {
		return fmt.Sprintf("Dify API error (HTTP %d, code %s): %s", e.StatusCode, e.Code, msg)
	}
	return fmt.Sprintf("Dify API error (HTTP %d): %s", e.StatusCode, msg)
}

// isNotFound reports whether err is an APIError with HTTP 404.
func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// isServiceAPIDisabled reports whether err is the server's 403
// `service_api_disabled` error — the app's Service API is off, which gates
// app management calls (DSL export, publish, versions).
func isServiceAPIDisabled(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden &&
		apiErr.Code == "service_api_disabled"
}

// apiErrorFromBody builds an APIError from a non-2xx response body,
// decoding the ErrorBody shape when present.
func apiErrorFromBody(statusCode int, data []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode, Body: string(data)}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}
	if err := json.Unmarshal(data, &body); err == nil {
		apiErr.Code = body.Code
		apiErr.Message = body.Message
	}
	return apiErr
}

// readResponse reads the whole body and, for non-2xx statuses, returns an APIError.
func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiErrorFromBody(resp.StatusCode, data)
	}

	return data, nil
}

// doJSON performs a request and unmarshals a 2xx JSON response into out.
func (c *DifyClient) doJSON(ctx context.Context, method, url string, reqBody, out any) error {
	resp, err := c.doRequest(ctx, method, url, reqBody)
	if err != nil {
		return err
	}

	data, err := readResponse(resp)
	if err != nil {
		return err
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(data))
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// App API
// ---------------------------------------------------------------------------

// AppImportRequest is the payload for POST /workspaces/{ws}/apps/imports.
type AppImportRequest struct {
	YAMLContent string `json:"yaml_content"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	AppID       string `json:"app_id,omitempty"`
	DraftHash   string `json:"draft_hash,omitempty"`
}

// ImportWarning is one portable DSL reference that could not be restored.
type ImportWarning struct {
	Code    string         `json:"code"`
	Path    string         `json:"path"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// ImportResult mirrors the Import shape returned by the DSL import endpoints.
// A failed import is answered with HTTP 400 carrying this same shape, not ErrorBody.
type ImportResult struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	AppID    string          `json:"app_id"`
	AppMode  string          `json:"app_mode"`
	Error    string          `json:"error"`
	Warnings []ImportWarning `json:"warnings"`
}

const (
	importStatusPending = "pending"
	importStatusFailed  = "failed"
)

// readImportResult decodes an import/confirm response. On HTTP 400 the body is
// the Import shape when the import itself failed, and ErrorBody otherwise.
func readImportResult(resp *http.Response) (*ImportResult, error) {
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusBadRequest {
		var result ImportResult
		if err := json.Unmarshal(data, &result); err == nil && result.Status == importStatusFailed {
			return nil, fmt.Errorf("DSL import failed: %s", result.Error)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiErrorFromBody(resp.StatusCode, data)
	}

	var result ImportResult
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(data))
		}
	}

	return &result, nil
}

// ImportApp creates (AppID empty) or overwrites (AppID set) an app from DSL
// YAML, confirming the import when the server answers pending. Both completed
// and completed-with-warnings are success; failed is an error.
func (c *DifyClient) ImportApp(ctx context.Context, req AppImportRequest) (*ImportResult, error) {
	payload := map[string]any{
		"mode":         "yaml-content",
		"yaml_content": req.YAMLContent,
	}
	if req.Name != "" {
		payload["name"] = req.Name
	}
	if req.Description != "" {
		payload["description"] = req.Description
	}
	if req.AppID != "" {
		payload["app_id"] = req.AppID
	}
	if req.DraftHash != "" {
		payload["draft_hash"] = req.DraftHash
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.workspaceURL("/apps/imports"), payload)
	if err != nil {
		return nil, err
	}

	result, err := readImportResult(resp)
	if err != nil {
		return nil, err
	}

	if result.Status == importStatusPending {
		result, err = c.confirmImport(ctx, result.ID)
		if err != nil {
			return nil, err
		}
	}

	if result.Status == importStatusFailed {
		return nil, fmt.Errorf("DSL import failed: %s", result.Error)
	}

	return result, nil
}

// confirmImport confirms a pending DSL import.
func (c *DifyClient) confirmImport(ctx context.Context, importID string) (*ImportResult, error) {
	url := c.workspaceURL("/apps/imports/" + url.PathEscape(importID) + ":confirm")
	resp, err := c.doRequest(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	result, err := readImportResult(resp)
	if err != nil {
		return nil, err
	}

	if result.Status == importStatusFailed {
		return nil, fmt.Errorf("DSL import failed: %s", result.Error)
	}

	return result, nil
}

// AppExport is the response of GET /apps/{app_id}/dsl.
type AppExport struct {
	Data      string `json:"data"`
	DraftHash string `json:"draft_hash"`
}

// ExportAppDSL exports an app's current draft DSL.
func (c *DifyClient) ExportAppDSL(ctx context.Context, appID string) (*AppExport, error) {
	url := fmt.Sprintf("%s/apps/%s/dsl", c.BaseURL, url.PathEscape(appID))
	var result AppExport
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteApp deletes an app; a 404 is reported as an APIError for the caller to ignore.
func (c *DifyClient) DeleteApp(ctx context.Context, appID string) error {
	url := fmt.Sprintf("%s/apps/%s", c.BaseURL, url.PathEscape(appID))
	return c.doJSON(ctx, http.MethodDelete, url, nil, nil)
}

// AppListItem is one row of GET /apps.
type AppListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
	UpdatedAt   string `json:"updated_at"`
}

type appListResponse struct {
	HasMore bool          `json:"has_more"`
	Data    []AppListItem `json:"data"`
}

// AccountResponse mirrors GET /openapi/v1/account — the fields the provider
// needs for workspace resolution.
type AccountResponse struct {
	DefaultWorkspaceID string `json:"default_workspace_id"`
}

// GetAccount returns the token owner's account info. The endpoint is
// unscoped (no workspace) and is reachable with a PAT.
func (c *DifyClient) GetAccount(ctx context.Context) (*AccountResponse, error) {
	var result AccountResponse
	if err := c.doJSON(ctx, http.MethodGet, c.BaseURL+"/account", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListApps returns the first page (limit 100) of apps whose name contains the
// given substring; an app id passed as name returns that one app.
func (c *DifyClient) ListApps(ctx context.Context, name string) ([]AppListItem, error) {
	query := url.Values{}
	query.Set("workspace_id", c.WorkspaceID)
	query.Set("page", "1")
	query.Set("limit", "100")
	if name != "" {
		query.Set("name", name)
	}

	url := fmt.Sprintf("%s/apps?%s", c.BaseURL, query.Encode())
	var result appListResponse
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// ---------------------------------------------------------------------------
// App API Key API
// ---------------------------------------------------------------------------

// AppAPIKey is one app API key row. created_at/last_used_at are unix seconds.
type AppAPIKey struct {
	ID         string `json:"id"`
	Token      string `json:"token"`
	CreatedAt  *int64 `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at"`
}

type appAPIKeyListResponse struct {
	Data []AppAPIKey `json:"data"`
}

// CreateAppAPIKey creates an API key for an app (201 with the full token).
func (c *DifyClient) CreateAppAPIKey(ctx context.Context, appID string) (*AppAPIKey, error) {
	url := fmt.Sprintf("%s/apps/%s/api-keys", c.BaseURL, url.PathEscape(appID))
	var result AppAPIKey
	if err := c.doJSON(ctx, http.MethodPost, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAppAPIKeys lists API keys for an app.
func (c *DifyClient) ListAppAPIKeys(ctx context.Context, appID string) ([]AppAPIKey, error) {
	url := fmt.Sprintf("%s/apps/%s/api-keys", c.BaseURL, url.PathEscape(appID))
	var result appAPIKeyListResponse
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// DeleteAppAPIKey deletes an API key for an app.
func (c *DifyClient) DeleteAppAPIKey(ctx context.Context, appID, keyID string) error {
	url := fmt.Sprintf("%s/apps/%s/api-keys/%s", c.BaseURL, url.PathEscape(appID), url.PathEscape(keyID))
	return c.doJSON(ctx, http.MethodDelete, url, nil, nil)
}

// ---------------------------------------------------------------------------
// Provider credentials (model and tool providers share the shape)
// ---------------------------------------------------------------------------

// CredentialRef is a saved credential reference (id + optional name).
type CredentialRef struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// CredentialWriteResult mirrors CredentialWriteResponse {id, name, active}.
type CredentialWriteResult struct {
	ID     string  `json:"id"`
	Name   *string `json:"name"`
	Active bool    `json:"active"`
}

type credentialWriteRequest struct {
	Credentials map[string]any `json:"credentials"`
	Name        string         `json:"name,omitempty"`
}

// CustomModelRef is one custom model of a provider, with its active credential.
type CustomModelRef struct {
	Model            string         `json:"model"`
	ModelType        string         `json:"model_type"`
	ActiveCredential *CredentialRef `json:"active_credential"`
}

type providerDescribeResponse struct {
	Credentials       []CredentialRef  `json:"credentials"`
	ActiveCredential  *CredentialRef   `json:"active_credential"`
	DefaultCredential *CredentialRef   `json:"default_credential"`
	CustomModels      []CustomModelRef `json:"custom_models"`
}

// credentialIDList returns the ids of a provider's saved credentials.
func credentialIDs(creds []CredentialRef) map[string]CredentialRef {
	out := make(map[string]CredentialRef, len(creds))
	for _, c := range creds {
		out[c.ID] = c
	}
	return out
}

// activeCredentialID returns the id of the credential the provider currently uses
// (active_credential for model providers, default_credential for tool providers).
func activeCredentialID(d *providerDescribeResponse) string {
	if d.ActiveCredential != nil {
		return d.ActiveCredential.ID
	}
	if d.DefaultCredential != nil {
		return d.DefaultCredential.ID
	}
	return ""
}

// credentialName returns the saved name for id, or "" when absent.
func credentialName(creds map[string]CredentialRef, id string) string {
	if ref, ok := creds[id]; ok && ref.Name != nil {
		return *ref.Name
	}
	return ""
}

func (c *DifyClient) describeProvider(ctx context.Context, kind, provider string) (*providerDescribeResponse, error) {
	// provider is a <path:provider> route segment (e.g. langgenius/openai/openai)
	// and must be interpolated unescaped so its slashes stay path separators.
	url := c.workspaceURL("/" + kind + "s/" + provider)
	var result providerDescribeResponse
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *DifyClient) writeCredential(ctx context.Context, method, kind, provider, credentialID string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	base := c.workspaceURL("/" + kind + "s/" + provider + "/credentials")
	if credentialID != "" {
		base += "/" + url.PathEscape(credentialID)
	}
	var result CredentialWriteResult
	if err := c.doJSON(ctx, method, base, credentialWriteRequest{Credentials: credentials, Name: name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *DifyClient) deleteCredential(ctx context.Context, kind, provider, credentialID string) error {
	url := c.workspaceURL("/" + kind + "s/" + provider + "/credentials/" + url.PathEscape(credentialID))
	return c.doJSON(ctx, http.MethodDelete, url, nil, nil)
}

// DescribeModelProvider returns the saved credentials of a model provider.
func (c *DifyClient) DescribeModelProvider(ctx context.Context, provider string) (*providerDescribeResponse, error) {
	return c.describeProvider(ctx, "model-provider", provider)
}

// CreateModelCredential saves a credential for a model provider (201).
func (c *DifyClient) CreateModelCredential(ctx context.Context, provider string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	return c.writeCredential(ctx, http.MethodPost, "model-provider", provider, "", credentials, name)
}

// UpdateModelCredential replaces a saved model provider credential (200).
func (c *DifyClient) UpdateModelCredential(ctx context.Context, provider, credentialID string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	return c.writeCredential(ctx, http.MethodPatch, "model-provider", provider, credentialID, credentials, name)
}

// DeleteModelCredential deletes a saved model provider credential.
func (c *DifyClient) DeleteModelCredential(ctx context.Context, provider, credentialID string) error {
	return c.deleteCredential(ctx, "model-provider", provider, credentialID)
}

// DescribeToolProvider returns the saved credentials of a tool provider.
func (c *DifyClient) DescribeToolProvider(ctx context.Context, provider string) (*providerDescribeResponse, error) {
	return c.describeProvider(ctx, "tool-provider", provider)
}

// CreateToolCredential saves a credential for a tool provider (201).
func (c *DifyClient) CreateToolCredential(ctx context.Context, provider string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	return c.writeCredential(ctx, http.MethodPost, "tool-provider", provider, "", credentials, name)
}

// UpdateToolCredential replaces a saved tool provider credential (200).
func (c *DifyClient) UpdateToolCredential(ctx context.Context, provider, credentialID string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	return c.writeCredential(ctx, http.MethodPatch, "tool-provider", provider, credentialID, credentials, name)
}

// DeleteToolCredential deletes a saved tool provider credential.
func (c *DifyClient) DeleteToolCredential(ctx context.Context, provider, credentialID string) error {
	return c.deleteCredential(ctx, "tool-provider", provider, credentialID)
}

// ---------------------------------------------------------------------------
// Plugin API
// ---------------------------------------------------------------------------

// PluginInstallStart is the response of POST /plugins:install.
type PluginInstallStart struct {
	TaskID       string `json:"task_id"`
	AllInstalled bool   `json:"all_installed"`
}

// PluginTaskPlugin is one plugin row inside a plugin task.
type PluginTaskPlugin struct {
	PluginID   string `json:"plugin_id"`
	Identifier string `json:"identifier"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

// PluginTask is the response of GET /plugin-tasks/{task_id}.
type PluginTask struct {
	TaskID  string             `json:"task_id"`
	Status  string             `json:"status"`
	Plugins []PluginTaskPlugin `json:"plugins"`
}

const (
	pluginTaskPending = "pending"
	pluginTaskRunning = "running"
	pluginTaskSuccess = "success"
	pluginTaskFailed  = "failed"
)

// pluginTask fetches the current state of a plugin install task.
func (c *DifyClient) pluginTask(ctx context.Context, taskID string) (*PluginTask, error) {
	url := c.workspaceURL("/plugin-tasks/" + url.PathEscape(taskID))
	var result PluginTask
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// waitForPluginTask polls a plugin install task until it succeeds or fails.
func (c *DifyClient) waitForPluginTask(ctx context.Context, taskID string) error {
	deadline := time.Now().Add(c.pluginPollTimeout)
	for {
		task, err := c.pluginTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to check plugin install task: %w", err)
		}

		switch task.Status {
		case pluginTaskSuccess:
			return nil
		case pluginTaskFailed:
			messages := make([]string, 0, len(task.Plugins))
			for _, p := range task.Plugins {
				if p.Message != "" {
					messages = append(messages, p.Message)
				}
			}
			if len(messages) == 0 {
				messages = append(messages, "unknown error")
			}
			return fmt.Errorf("plugin install task %s failed: %s", taskID, strings.Join(messages, "; "))
		}

		if !time.Now().Before(deadline) {
			return fmt.Errorf("plugin install task %s did not finish within %s", taskID, c.pluginPollTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pluginPollInterval):
		}
	}
}

// InstallPlugin starts a marketplace plugin install and waits for the task to
// finish when the server reports the install is still in flight.
func (c *DifyClient) InstallPlugin(ctx context.Context, identifier string) error {
	url := c.workspaceURL("/plugins:install")
	var start PluginInstallStart
	if err := c.doJSON(ctx, http.MethodPost, url, map[string]any{
		"identifiers": []string{identifier},
	}, &start); err != nil {
		return err
	}

	if start.AllInstalled {
		return nil
	}
	return c.waitForPluginTask(ctx, start.TaskID)
}

// PluginInfo is one installed plugin row.
type PluginInfo struct {
	PluginID   string `json:"plugin_id"`
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
	Category   string `json:"category"`
	Source     string `json:"source"`
}

type pluginListResponse struct {
	Data []PluginInfo `json:"data"`
}

// ListPlugins lists plugins installed in the workspace.
func (c *DifyClient) ListPlugins(ctx context.Context) ([]PluginInfo, error) {
	var result pluginListResponse
	if err := c.doJSON(ctx, http.MethodGet, c.workspaceURL("/plugins"), nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// UninstallPlugin uninstalls a plugin by plugin_id (e.g. langgenius/openai).
// plugin_id is a <path:plugin_id> route segment and is interpolated unescaped.
func (c *DifyClient) UninstallPlugin(ctx context.Context, pluginID string) error {
	url := c.workspaceURL("/plugins/" + pluginID)
	return c.doJSON(ctx, http.MethodDelete, url, nil, nil)
}

// ---------------------------------------------------------------------------
// Models API (data source)
// ---------------------------------------------------------------------------

// ModelInfo is one model row of GET /workspaces/{ws}/models.
type ModelInfo struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	ModelType string `json:"model_type"`
	Status    string `json:"status"`
}

type modelListResponse struct {
	HasMore bool        `json:"has_more"`
	Data    []ModelInfo `json:"data"`
}

// ListModels returns every model in the workspace, following pagination.
// provider and modelType are optional filters; empty means no filter.
func (c *DifyClient) ListModels(ctx context.Context, provider, modelType string) ([]ModelInfo, error) {
	var all []ModelInfo
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", "100")
		if provider != "" {
			query.Set("provider", provider)
		}
		if modelType != "" {
			query.Set("model_type", modelType)
		}

		url := c.workspaceURL("/models") + "?" + query.Encode()
		var result modelListResponse
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
			return nil, err
		}

		all = append(all, result.Data...)
		if !result.HasMore {
			return all, nil
		}
	}
}

// ---------------------------------------------------------------------------
// App publish and Service API
// ---------------------------------------------------------------------------

// PublishResult is the response of POST /apps/{app_id}:publish.
type PublishResult struct {
	VersionID string  `json:"version_id"`
	CreatedAt int64   `json:"created_at"`
	Warning   *string `json:"warning"`
}

// PublishApp publishes the app's current draft as a new version and makes the
// app use it. markedName/markedComment optionally name the version.
func (c *DifyClient) PublishApp(ctx context.Context, appID, markedName, markedComment string) (*PublishResult, error) {
	url := fmt.Sprintf("%s/apps/%s:publish", c.BaseURL, url.PathEscape(appID))
	var result PublishResult
	if err := c.doJSON(ctx, http.MethodPost, url, map[string]any{
		"marked_name":    markedName,
		"marked_comment": markedComment,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ServiceAPI is the response of the service-api describe/set endpoints
// (GET/PATCH /apps/{app_id}/service-api or /service-api/agent).
type ServiceAPI struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
}

// serviceAPIURL returns the app's service-api route; agent apps use the
// dedicated /service-api/agent variant.
func (c *DifyClient) serviceAPIURL(appID string, agent bool) string {
	suffix := "/service-api"
	if agent {
		suffix += "/agent"
	}
	return fmt.Sprintf("%s/apps/%s%s", c.BaseURL, url.PathEscape(appID), suffix)
}

// GetServiceAPI reads whether the app's Service API is enabled.
func (c *DifyClient) GetServiceAPI(ctx context.Context, appID string, agent bool) (*ServiceAPI, error) {
	var result ServiceAPI
	if err := c.doJSON(ctx, http.MethodGet, c.serviceAPIURL(appID, agent), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetServiceAPI turns the app's Service API on or off.
func (c *DifyClient) SetServiceAPI(ctx context.Context, appID string, agent, enabled bool) (*ServiceAPI, error) {
	var result ServiceAPI
	if err := c.doJSON(ctx, http.MethodPatch, c.serviceAPIURL(appID, agent), map[string]any{
		"enabled": enabled,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ActivateModelProviderCredential makes a saved model provider credential the
// active one (POST {provider}/credentials/{credential_id}:activate).
func (c *DifyClient) ActivateModelProviderCredential(ctx context.Context, provider, credentialID string) (*CredentialWriteResult, error) {
	url := c.workspaceURL("/model-providers/" + provider + "/credentials/" + url.PathEscape(credentialID) + ":activate")
	var result CredentialWriteResult
	if err := c.doJSON(ctx, http.MethodPost, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Custom-model credentials (model-providers/{provider}/models/credentials)
// ---------------------------------------------------------------------------

type modelCredentialRequest struct {
	Model       string         `json:"model"`
	ModelType   string         `json:"model_type"`
	Credentials map[string]any `json:"credentials"`
	Name        string         `json:"name,omitempty"`
}

// CreateModelScopedCredential saves a credential for one model of a provider
// (201). The first credential of a model not yet set up becomes its active one.
func (c *DifyClient) CreateModelScopedCredential(ctx context.Context, provider, model, modelType string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	url := c.workspaceURL("/model-providers/" + provider + "/models/credentials")
	var result CredentialWriteResult
	if err := c.doJSON(ctx, http.MethodPost, url, modelCredentialRequest{
		Model:       model,
		ModelType:   modelType,
		Credentials: credentials,
		Name:        name,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateModelScopedCredential replaces a saved credential of one model (200).
func (c *DifyClient) UpdateModelScopedCredential(ctx context.Context, provider, model, modelType, credentialID string, credentials map[string]any, name string) (*CredentialWriteResult, error) {
	url := c.workspaceURL("/model-providers/" + provider + "/models/credentials/" + url.PathEscape(credentialID))
	var result CredentialWriteResult
	if err := c.doJSON(ctx, http.MethodPatch, url, modelCredentialRequest{
		Model:       model,
		ModelType:   modelType,
		Credentials: credentials,
		Name:        name,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteModelScopedCredential deletes a saved credential of one model;
// model and model_type travel as required query parameters.
func (c *DifyClient) DeleteModelScopedCredential(ctx context.Context, provider, model, modelType, credentialID string) error {
	query := url.Values{}
	query.Set("model", model)
	query.Set("model_type", modelType)
	url := c.workspaceURL("/model-providers/"+provider+"/models/credentials/"+url.PathEscape(credentialID)) + "?" + query.Encode()
	return c.doJSON(ctx, http.MethodDelete, url, nil, nil)
}

// ActivateModelScopedCredential makes a saved credential the active one for
// one model (POST {provider}/models/credentials/{credential_id}:activate).
func (c *DifyClient) ActivateModelScopedCredential(ctx context.Context, provider, model, modelType, credentialID string) (*CredentialWriteResult, error) {
	url := c.workspaceURL("/model-providers/" + provider + "/models/credentials/" + url.PathEscape(credentialID) + ":activate")
	var result CredentialWriteResult
	if err := c.doJSON(ctx, http.MethodPost, url, map[string]any{
		"model":      model,
		"model_type": modelType,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Default models (workspaces/{ws}/default-models/{model_type})
// ---------------------------------------------------------------------------

// DefaultModel is the response of the default-model describe/set endpoints.
type DefaultModel struct {
	ModelType string  `json:"model_type"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
}

// GetDefaultModel reads the workspace's default model of one model type;
// Provider and Model are nil while no default is set.
func (c *DifyClient) GetDefaultModel(ctx context.Context, modelType string) (*DefaultModel, error) {
	url := c.workspaceURL("/default-models/" + url.PathEscape(modelType))
	var result DefaultModel
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetDefaultModel sets the workspace's default model of one model type.
func (c *DifyClient) SetDefaultModel(ctx context.Context, modelType, provider, model string) (*DefaultModel, error) {
	url := c.workspaceURL("/default-models/" + url.PathEscape(modelType))
	var result DefaultModel
	if err := c.doJSON(ctx, http.MethodPut, url, map[string]any{
		"provider": provider,
		"model":    model,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Model settings (model-providers/{provider}/models/settings)
// ---------------------------------------------------------------------------

// LoadBalancingEntry is one stored load-balancing entry of a model.
type LoadBalancingEntry struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CredentialID *string `json:"credential_id"`
	Enabled      bool    `json:"enabled"`
	Inherit      bool    `json:"inherit"`
}

// ModelSettings is the response of the model-settings describe/set endpoints.
type ModelSettings struct {
	Model         string `json:"model"`
	ModelType     string `json:"model_type"`
	ConfigFrom    string `json:"config_from"`
	Enabled       bool   `json:"enabled"`
	LoadBalancing struct {
		Enabled bool                 `json:"enabled"`
		Entries []LoadBalancingEntry `json:"entries"`
	} `json:"load_balancing"`
}

// LoadBalancingEntryRequest is one desired load-balancing entry; a nil
// CredentialID names the entry that reuses the active credential.
type LoadBalancingEntryRequest struct {
	CredentialID *string `json:"credential_id"`
	Enabled      bool    `json:"enabled"`
}

// GetModelSettings reads a model's enablement and load-balancing settings.
// configFrom selects which credential table the entries reference
// ("predefined-model" or "custom-model").
func (c *DifyClient) GetModelSettings(ctx context.Context, provider, model, modelType, configFrom string) (*ModelSettings, error) {
	query := url.Values{}
	query.Set("model", model)
	query.Set("model_type", modelType)
	query.Set("config_from", configFrom)
	url := c.workspaceURL("/model-providers/"+provider+"/models/settings") + "?" + query.Encode()
	var result ModelSettings
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetModelSettings replaces a model's enablement and load-balancing settings;
// entries are declarative — stored entries not listed are deleted.
func (c *DifyClient) SetModelSettings(ctx context.Context, provider, model, modelType, configFrom string, enabled, lbEnabled bool, entries []LoadBalancingEntryRequest) (*ModelSettings, error) {
	url := c.workspaceURL("/model-providers/" + provider + "/models/settings")
	var result ModelSettings
	if err := c.doJSON(ctx, http.MethodPut, url, map[string]any{
		"model":       model,
		"model_type":  modelType,
		"config_from": configFrom,
		"enabled":     enabled,
		"load_balancing": map[string]any{
			"enabled": lbEnabled,
			"entries": entries,
		},
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
