// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withCatalog wraps handler so that GET /openapi/v1/_version returns the given
// fingerprint in the X-Dify-Catalog response header, as the real server does.
// versionHits, when non-nil, counts /_version requests.
func withCatalog(versionHits *atomic.Int32, fingerprint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/openapi/v1/_version" {
			if versionHits != nil {
				versionHits.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Dify-Catalog", fingerprint)
			_, _ = w.Write([]byte(`{"version":"test"}`))
			return
		}
		next(w, r)
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*DifyClient, *httptest.Server, *atomic.Int32) {
	t.Helper()
	var versionHits atomic.Int32
	srv := httptest.NewServer(withCatalog(&versionHits, "fp-1", handler))
	t.Cleanup(srv.Close)
	return NewDifyClient(srv.URL, "test-token", "ws-1"), srv, &versionHits
}

func TestBearerHeaderAndBasePath(t *testing.T) {
	var gotAuth, gotCatalog, gotPath string
	client, _, versionHits := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCatalog = r.Header.Get("X-Dify-Catalog")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dify-Catalog", "fp-1")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	if _, err := client.ListPlugins(context.Background()); err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected bearer token header, got %q", gotAuth)
	}
	if gotPath != "/openapi/v1/workspaces/ws-1/plugins" {
		t.Fatalf("expected /openapi/v1 base path, got %q", gotPath)
	}
	if gotCatalog != "fp-1" {
		t.Fatalf("expected X-Dify-Catalog fp-1, got %q", gotCatalog)
	}
	if versionHits.Load() != 1 {
		t.Fatalf("expected one /_version fetch, got %d", versionHits.Load())
	}
}

func TestCatalogStaleRetriesOnce(t *testing.T) {
	var attempts atomic.Int32
	var retriedCatalog string
	client, _, versionHits := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts.Add(1) == 1 {
			w.Header().Set("X-Dify-Catalog", "fp-2")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"code":"catalog_stale","message":"catalog changed","status":412}`))
			return
		}
		retriedCatalog = r.Header.Get("X-Dify-Catalog")
		w.Header().Set("X-Dify-Catalog", "fp-2")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	if _, err := client.ListPlugins(context.Background()); err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts.Load())
	}
	if retriedCatalog != "fp-2" {
		t.Fatalf("expected retry with fingerprint fp-2, got %q", retriedCatalog)
	}
	if versionHits.Load() != 1 {
		t.Fatalf("expected fingerprint from 412 header, no /_version refetch; got %d fetches", versionHits.Load())
	}
}

func TestCatalogStalePersistentReturnsError(t *testing.T) {
	var attempts atomic.Int32
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		attempts.Add(1)
		w.Header().Set("X-Dify-Catalog", "fp-2")
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte(`{"code":"catalog_stale","message":"still stale","status":412}`))
	})

	_, err := client.ListPlugins(context.Background())
	if err == nil {
		t.Fatal("expected error for persistent 412 catalog_stale")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("expected status 412, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "catalog_stale" {
		t.Fatalf("expected code catalog_stale, got %q", apiErr.Code)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected exactly one retry (2 attempts), got %d", attempts.Load())
	}
}

func TestRateLimitRetryAfterHeader(t *testing.T) {
	var attempts atomic.Int32
	var sleptFor time.Duration
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"too_many_requests","message":"slow down","status":429}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	client.sleep = func(_ context.Context, d time.Duration) error {
		sleptFor = d
		return nil
	}

	if _, err := client.ListPlugins(context.Background()); err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
	if sleptFor != 2*time.Second {
		t.Fatalf("expected sleep of 2s from Retry-After, got %v", sleptFor)
	}
}

func TestRateLimitRetriesExhausted(t *testing.T) {
	var attempts, sleeps atomic.Int32
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"too_many_requests","message":"slow down","status":429}`))
	})
	client.sleep = func(_ context.Context, _ time.Duration) error {
		sleeps.Add(1)
		return nil
	}

	_, err := client.ListPlugins(context.Background())
	if err == nil {
		t.Fatal("expected error after rate-limit retries exhausted")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "too_many_requests" {
		t.Fatalf("expected code too_many_requests, got %q", apiErr.Code)
	}
	if attempts.Load() != 6 {
		t.Fatalf("expected 6 attempts (1 + 5 retries), got %d", attempts.Load())
	}
	if sleeps.Load() != 5 {
		t.Fatalf("expected 5 sleeps, got %d", sleeps.Load())
	}
}

func TestRateLimitWaitHonoursContextCancel(t *testing.T) {
	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		attempts.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"too_many_requests","message":"slow down","status":429}`))
	})
	client.sleep = func(sctx context.Context, _ time.Duration) error {
		cancel()
		return sctx.Err()
	}

	_, err := client.ListPlugins(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected no retry after cancelled wait, got %d attempts", attempts.Load())
	}
}

func TestAPIErrorParsesErrorBody(t *testing.T) {
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found","message":"App not found","status":404}`))
	})

	_, err := client.ExportAppDSL(context.Background(), "app-1")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "not_found" {
		t.Fatalf("expected code not_found, got %q", apiErr.Code)
	}
	if apiErr.Message != "App not found" {
		t.Fatalf("expected message 'App not found', got %q", apiErr.Message)
	}
	if !isNotFound(err) {
		t.Fatal("expected isNotFound to be true")
	}
}

func TestImportFailedBadRequest(t *testing.T) {
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"id":"imp-1","status":"failed","app_id":"","app_mode":"","error":"invalid DSL: missing version"}`))
	})

	_, err := client.ImportApp(context.Background(), AppImportRequest{YAMLContent: "bad"})
	if err == nil {
		t.Fatal("expected error for failed import")
	}
	if !strings.Contains(err.Error(), "invalid DSL: missing version") {
		t.Fatalf("expected import error text, got: %v", err)
	}
}

func TestImportPendingTriggersConfirm(t *testing.T) {
	var confirmed atomic.Bool
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v1/workspaces/ws-1/apps/imports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"imp-9","status":"pending","app_id":"","app_mode":""}`))
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v1/workspaces/ws-1/apps/imports/imp-9:confirm":
			confirmed.Store(true)
			_, _ = w.Write([]byte(`{"id":"imp-9","status":"completed","app_id":"app-9","app_mode":"workflow"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"unexpected path","status":404}`))
		}
	})

	result, err := client.ImportApp(context.Background(), AppImportRequest{YAMLContent: "app: {}"})
	if err != nil {
		t.Fatalf("ImportApp failed: %v", err)
	}
	if !confirmed.Load() {
		t.Fatal("expected confirm endpoint to be called for pending import")
	}
	if result.AppID != "app-9" {
		t.Fatalf("expected app_id app-9, got %q", result.AppID)
	}
}

func TestInstallPluginPollsTaskToSuccess(t *testing.T) {
	var taskCalls atomic.Int32
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v1/workspaces/ws-1/plugins:install":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			ids, _ := body["identifiers"].([]any)
			if len(ids) != 1 || ids[0] != "langgenius/openai:0.3.8@abc" {
				t.Errorf("unexpected install identifiers: %v", body["identifiers"])
			}
			_, _ = w.Write([]byte(`{"task_id":"task-1","all_installed":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/openapi/v1/workspaces/ws-1/plugin-tasks/task-1":
			n := taskCalls.Add(1)
			if n < 2 {
				_, _ = w.Write([]byte(`{"task_id":"task-1","status":"running","plugins":[]}`))
			} else {
				_, _ = w.Write([]byte(`{"task_id":"task-1","status":"success","plugins":[{"plugin_id":"langgenius/openai","identifier":"langgenius/openai:0.3.8@abc","status":"success","message":"installed"}]}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	client.pluginPollInterval = time.Millisecond

	if err := client.InstallPlugin(context.Background(), "langgenius/openai:0.3.8@abc"); err != nil {
		t.Fatalf("InstallPlugin failed: %v", err)
	}
	if taskCalls.Load() < 2 {
		t.Fatalf("expected task polling, got %d calls", taskCalls.Load())
	}
}

func TestInstallPluginTaskFailedSurfacesMessages(t *testing.T) {
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plugins:install"):
			_, _ = w.Write([]byte(`{"task_id":"task-2","all_installed":false}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plugin-tasks/task-2"):
			_, _ = w.Write([]byte(`{"task_id":"task-2","status":"failed","plugins":[{"plugin_id":"langgenius/openai","identifier":"langgenius/openai:0.3.8@abc","status":"failed","message":"checksum mismatch"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	client.pluginPollInterval = time.Millisecond

	err := client.InstallPlugin(context.Background(), "langgenius/openai:0.3.8@abc")
	if err == nil {
		t.Fatal("expected error for failed plugin task")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected plugin failure message, got: %v", err)
	}
}

func TestGetAccount(t *testing.T) {
	var gotMethod, gotPath string
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dify-Catalog", "fp-1")
		_, _ = w.Write([]byte(`{"subject_type":"personal_access_token","default_workspace_id":"ws-default"}`))
	})

	account, err := client.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("expected GET, got %s", gotMethod)
	}
	if gotPath != "/openapi/v1/account" {
		t.Fatalf("expected /openapi/v1/account, got %q", gotPath)
	}
	if account.DefaultWorkspaceID != "ws-default" {
		t.Fatalf("expected ws-default, got %q", account.DefaultWorkspaceID)
	}
}

func TestResolveWorkspaceIDOrder(t *testing.T) {
	accountHits := 0
	client, _, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		accountHits++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dify-Catalog", "fp-1")
		_, _ = w.Write([]byte(`{"default_workspace_id":"ws-from-account"}`))
	})

	// Configured value wins and never calls /account.
	t.Setenv("DIFY_WORKSPACE_ID", "ws-from-env")
	got, err := resolveWorkspaceID(context.Background(), client, "ws-from-config")
	if err != nil || got != "ws-from-config" {
		t.Fatalf("config: got %q err %v", got, err)
	}
	if accountHits != 0 {
		t.Fatalf("config path must not call /account, got %d calls", accountHits)
	}

	// Env wins over the account lookup when config is unset.
	got, err = resolveWorkspaceID(context.Background(), client, "")
	if err != nil || got != "ws-from-env" {
		t.Fatalf("env: got %q err %v", got, err)
	}
	if accountHits != 0 {
		t.Fatalf("env path must not call /account, got %d calls", accountHits)
	}

	// Otherwise the token owner's default workspace is used.
	t.Setenv("DIFY_WORKSPACE_ID", "")
	got, err = resolveWorkspaceID(context.Background(), client, "")
	if err != nil || got != "ws-from-account" {
		t.Fatalf("account: got %q err %v", got, err)
	}
	if accountHits != 1 {
		t.Fatalf("expected one /account call, got %d", accountHits)
	}
}
