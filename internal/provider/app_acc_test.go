// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// accEnv returns the acceptance-test credentials, skipping when TF_ACC is not
// enabled and failing (never silently skipping) when live credentials are
// missing under TF_ACC=1.
func accEnv(t *testing.T) (host, token string) {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("TF_ACC is not set; skipping acceptance test")
	}
	host = os.Getenv("DIFY_HOST")
	token = os.Getenv("DIFY_TOKEN")
	if host == "" || token == "" {
		t.Fatal("TF_ACC=1 requires live DIFY_HOST and DIFY_TOKEN")
	}
	return host, token
}

// accScenariosDir is the source of truth for acceptance Terraform
// configurations; each scenario lives in its own directory.
const accScenariosDir = "testdata/acceptance"

// copyAccScenario copies the files of scenario testdata/acceptance/<name>
// into destination, which must already exist. Terraform is only ever run in
// the copy so state, lockfiles and plans never pollute the source tree.
func copyAccScenario(t *testing.T, name, destination string) {
	t.Helper()
	src := filepath.Join(accScenariosDir, name)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading scenario %q: %v", name, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(destination, e.Name())
		if e.IsDir() {
			if err := filepath.WalkDir(srcPath, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, err := filepath.Rel(srcPath, path)
				if err != nil {
					return err
				}
				target := filepath.Join(dstPath, rel)
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return os.WriteFile(target, data, 0o600)
			}); err != nil {
				t.Fatalf("copying scenario dir %s: %v", e.Name(), err)
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("reading scenario file %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(dstPath, data, 0o600); err != nil {
			t.Fatalf("copying scenario file %s: %v", e.Name(), err)
		}
	}
}

// terraformEnv builds a clean environment for terraform CLI subprocesses:
// credentials come from DIFY_HOST/DIFY_TOKEN and DIFY_WORKSPACE_ID is removed
// so the provider exercises the /openapi/v1/account lookup.
func terraformEnv(cliConfig string) []string {
	env := os.Environ()
	filtered := env[:0]
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		// Strip anything that could redirect the workspace or hijack the CLI:
		// workspace pinning, Terraform data dir, and global CLI args.
		if key == "DIFY_WORKSPACE_ID" || key == "TF_VAR_dify_workspace_id" ||
			key == "TF_DATA_DIR" || key == "TF_CLI_ARGS" ||
			strings.HasPrefix(key, "TF_CLI_ARGS_") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return append(filtered,
		"TF_CLI_CONFIG_FILE="+cliConfig,
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
	)
}

// tfResult captures one terraform CLI invocation.
type tfResult struct {
	Stdout string
	Stderr string
	Code   int
}

// secretEnvKeys are environment variable names whose values must never appear
// in captured terraform output or test logs.
var secretEnvKeys = []string{
	"DIFY_TOKEN",
	"TF_VAR_dify_token",
	"TF_VAR_openai_api_key",
	"TF_VAR_openai_api_key_backup",
	"TF_VAR_tavily_api_key",
	"TF_VAR_mcp_server_url",
}

func runTerraform(ctx context.Context, dir string, env []string, args ...string) tfResult {
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := tfResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.Code = exitErr.ExitCode()
		} else {
			res.Code = -1
			res.Stderr += "\n" + err.Error()
		}
	}
	// Redact any credential value that slipped into terraform output.
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || value == "" {
			continue
		}
		for _, secretKey := range secretEnvKeys {
			if key == secretKey {
				res.Stdout = strings.ReplaceAll(res.Stdout, value, "***")
				res.Stderr = strings.ReplaceAll(res.Stderr, value, "***")
			}
		}
	}
	return res
}

// mustTF runs terraform and fails the test on a non-zero exit.
func mustTF(t *testing.T, ctx context.Context, dir string, env []string, args ...string) tfResult {
	t.Helper()
	res := runTerraform(ctx, dir, env, args...)
	if res.Code != 0 {
		t.Fatalf("terraform %s failed (exit %d)\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), res.Code, res.Stdout, res.Stderr)
	}
	return res
}

// buildProviderMirror compiles the provider into a Terraform filesystem
// mirror layout and returns (cliConfigPath, scratchDir).
func buildProviderMirror(t *testing.T) (string, string) {
	t.Helper()

	scratch := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving module root: %v", err)
	}

	binName := "terraform-provider-dify"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binDir := filepath.Join(scratch, "mirror", "registry.terraform.io",
		"wylswz", "dify", "0.0.1",
		fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("creating mirror dir: %v", err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(binDir, binName))
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build provider: %v\n%s", err, out)
	}

	cliConfig := filepath.Join(scratch, "terraform.rc")
	rc := fmt.Sprintf(`provider_installation {
  filesystem_mirror {
    path = %q
  }
}
`, filepath.Join(scratch, "mirror"))
	if err := os.WriteFile(cliConfig, []byte(rc), 0o600); err != nil {
		t.Fatalf("writing CLI config: %v", err)
	}
	return cliConfig, scratch
}

// tfOutput reads one known-nonsecret string output via `terraform output -raw`.
// Deliberately not `output -json`, which would materialize sensitive outputs
// into the captured result (and into failure logs).
func tfOutput(t *testing.T, ctx context.Context, dir string, env []string, key string) string {
	t.Helper()
	res := mustTF(t, ctx, dir, env, "output", "-raw", key)
	return strings.TrimSpace(res.Stdout)
}

// tfOutputJSON returns the JSON encoding of a complex-typed output value.
func tfOutputJSON(t *testing.T, ctx context.Context, dir string, env []string, key string) string {
	t.Helper()
	res := mustTF(t, ctx, dir, env, "output", "-json", key)
	return strings.TrimSpace(res.Stdout)
}

func TestAccAppLifecycle(t *testing.T) {
	host, token := accEnv(t)

	cliConfig, scratch := buildProviderMirror(t)
	env := terraformEnv(cliConfig)
	ctx := context.Background()

	appName := fmt.Sprintf("tfacc-%d", time.Now().UnixNano())
	vars := func(name, description string) []string {
		return []string{
			"-var=app_name=" + name,
			"-var=app_description=" + description,
		}
	}

	dir := filepath.Join(scratch, "app_lifecycle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyAccScenario(t, "app_lifecycle", dir)

	// Registered before the first apply so a mid-test failure still destroys
	// whatever was created remotely. Bounded separately from the test ctx.
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		args := append([]string{"destroy", "-auto-approve", "-input=false"}, vars(appName, "")...)
		//nolint:errcheck // best-effort cleanup; failure is reported by the run.
		runTerraform(cleanCtx, dir, env, args...)
	})

	mustTF(t, ctx, dir, env, "init", "-input=false")

	// service_api_enabled = false must be rejected at validation time.
	badDir := filepath.Join(scratch, "invalid_service_api")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyAccScenario(t, "invalid_service_api", badDir)
	mustTF(t, ctx, badDir, env, "init", "-input=false")
	res := runTerraform(ctx, badDir, env, "plan", "-input=false", "-detailed-exitcode",
		"-var=app_name="+appName)
	if res.Code == 0 || res.Code == 2 {
		t.Fatalf("expected plan to fail for service_api_enabled=false, exit %d\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout+res.Stderr, "Disabling the App Service API") {
		t.Fatalf("expected validation diagnostic for service_api_enabled=false\nstdout:\n%s\nstderr:\n%s",
			res.Stdout, res.Stderr)
	}

	applyArgs := append([]string{"apply", "-auto-approve", "-input=false"}, vars(appName, "acceptance test")...)
	mustTF(t, ctx, dir, env, applyArgs...)
	appID := tfOutput(t, ctx, dir, env, "app_id")
	if appID == "" {
		t.Fatal("app_id output is empty after apply")
	}
	if mode := tfOutput(t, ctx, dir, env, "app_mode"); mode != "workflow" {
		t.Fatalf("expected mode workflow, got %q", mode)
	}
	if name := tfOutput(t, ctx, dir, env, "app_name"); name != appName {
		t.Fatalf("expected name %q, got %q", appName, name)
	}
	if v := tfOutput(t, ctx, dir, env, "service_api_enabled"); v != "true" {
		t.Fatalf("expected service_api_enabled \"true\", got %q", v)
	}

	// No-op plan: remote matches configuration.
	planArgs := append([]string{"plan", "-detailed-exitcode", "-input=false"}, vars(appName, "acceptance test")...)
	res = runTerraform(ctx, dir, env, planArgs...)
	if res.Code != 0 {
		t.Fatalf("expected empty plan after apply (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}

	// Idempotent update: name/description change keeps the same app ID.
	applyArgs = append([]string{"apply", "-auto-approve", "-input=false"}, vars(appName+"-v2", "updated by acceptance test")...)
	mustTF(t, ctx, dir, env, applyArgs...)
	if id := tfOutput(t, ctx, dir, env, "app_id"); id != appID {
		t.Fatalf("app id changed across update: %q -> %q", appID, id)
	}
	if name := tfOutput(t, ctx, dir, env, "app_name"); name != appName+"-v2" {
		t.Fatalf("expected updated name, got %q", name)
	}
	planArgs = append([]string{"plan", "-detailed-exitcode", "-input=false"}, vars(appName+"-v2", "updated by acceptance test")...)
	res = runTerraform(ctx, dir, env, planArgs...)
	if res.Code != 0 {
		t.Fatalf("expected empty plan after update (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}

	// Import: re-import the same remote app into a fresh state. The plan must
	// keep the same id (dsl_yaml is desired state — it is not compared to the
	// server's normalized exported_dsl_yaml).
	mustTF(t, ctx, dir, env, "state", "rm", "dify_app.test")
	mustTF(t, ctx, dir, env, "state", "rm", "dify_app_api_key.test")
	importArgs := append([]string{"import", "-input=false"}, vars(appName+"-v2", "updated by acceptance test")...)
	res = runTerraform(ctx, dir, env, append(importArgs, "dify_app.test", appID)...)
	if res.Code != 0 {
		t.Fatalf("terraform import failed (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}
	res = runTerraform(ctx, dir, env, append([]string{"plan", "-detailed-exitcode", "-input=false"}, vars(appName+"-v2", "updated by acceptance test")...)...)
	if res.Code != 2 {
		t.Fatalf("expected plan to show an in-place update after import (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "must be replaced") {
		t.Fatalf("imported workflow app must update in place, not replace\n%s", res.Stdout)
	}
	mustTF(t, ctx, dir, env, applyArgs...)
	if id := tfOutput(t, ctx, dir, env, "app_id"); id != appID {
		t.Fatalf("app id changed after import+apply: %q -> %q", appID, id)
	}

	// Destroy, then confirm the app is gone remotely.
	destroyArgs := append([]string{"destroy", "-auto-approve", "-input=false"}, vars(appName+"-v2", "updated by acceptance test")...)
	mustTF(t, ctx, dir, env, destroyArgs...)

	client := NewDifyClient(host, token, "")
	_, err := client.ExportAppDSL(ctx, appID)
	if err == nil {
		t.Fatalf("app %q still exists after destroy", appID)
	}
	if !isNotFound(err) {
		t.Fatalf("expected 404 after destroy, got: %v", err)
	}
}

// TestAccTools covers workflow-tool publish/lookup and the API tool provider
// against a static OpenAPI document — no outbound HTTP invocation.
func TestAccTools(t *testing.T) {
	accEnv(t)

	cliConfig, scratch := buildProviderMirror(t)
	env := terraformEnv(cliConfig)
	ctx := context.Background()

	appName := fmt.Sprintf("tftools-%d", time.Now().UnixNano())
	varArgs := []string{"-var=app_name=" + appName}

	dir := filepath.Join(scratch, "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyAccScenario(t, "tools", dir)

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		args := append([]string{"destroy", "-auto-approve", "-input=false"}, varArgs...)
		//nolint:errcheck // best-effort cleanup; failure is reported by the run.
		runTerraform(cleanCtx, dir, env, args...)
	})

	mustTF(t, ctx, dir, env, "init", "-input=false")
	mustTF(t, ctx, dir, env, append([]string{"apply", "-auto-approve", "-input=false"}, varArgs...)...)

	if id := tfOutput(t, ctx, dir, env, "workflow_tool_id"); id == "" {
		t.Fatal("workflow_tool_id output is empty")
	}
	if names := tfOutput(t, ctx, dir, env, "workflow_tool_names"); !strings.Contains(names, "acc_workflow_tool") {
		t.Fatalf("dify_tools lookup missing workflow tool name, got %q", names)
	}
	if name := tfOutput(t, ctx, dir, env, "api_tool_provider_name"); name != "acc_api_tools" {
		t.Fatalf("expected api_tool_provider_name acc_api_tools, got %q", name)
	}
	if names := tfOutput(t, ctx, dir, env, "api_tool_names"); !strings.Contains(names, "get_echo") {
		t.Fatalf("dify_tools lookup missing API tool operation, got %q", names)
	}

	res := runTerraform(ctx, dir, env, append([]string{"plan", "-detailed-exitcode", "-input=false"}, varArgs...)...)
	if res.Code != 0 {
		t.Fatalf("expected empty plan after apply (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}

	mustTF(t, ctx, dir, env, append([]string{"destroy", "-auto-approve", "-input=false"}, varArgs...)...)
}

// envWithout returns env with the named keys removed, so test-injected
// replacements appended afterwards are the ones the child process sees.
func envWithout(env []string, keys ...string) []string {
	out := env[:0]
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, k := range keys {
			if key == k {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// TestAccAppCreatePartialFailure drives the real Terraform CLI against an
// httptest fake where the DSL import succeeds (remote id app-42) but the
// follow-up export fails. Create must persist the remote id as partial
// state — the app exists remotely and must stay destroyable.
func TestAccAppCreatePartialFailure(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("TF_ACC is not set; skipping acceptance test")
	}

	var exportHits int
	srv := httptest.NewServer(withCatalog(nil, "fp-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Dify-Catalog", "fp-1")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/apps/imports"):
			_ = json.NewEncoder(w).Encode(ImportResult{Status: "completed", AppID: "app-42", AppMode: "workflow"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/apps/app-42/dsl"):
			exportHits++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"nope"}`))
		}
	}))
	t.Cleanup(srv.Close)

	cliConfig, scratch := buildProviderMirror(t)
	env := terraformEnv(cliConfig)
	// Redirect the provider at the fake — never the real credentials.
	env = append(envWithout(env, "DIFY_HOST", "DIFY_TOKEN"),
		"DIFY_HOST="+srv.URL,
		"DIFY_TOKEN=fake-token",
		"DIFY_WORKSPACE_ID=ws-1",
	)

	dir := filepath.Join(scratch, "app_lifecycle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyAccScenario(t, "app_lifecycle", dir)

	ctx := context.Background()
	mustTF(t, ctx, dir, env, "init", "-input=false")

	appName := fmt.Sprintf("tfpartial-%d", time.Now().UnixNano())
	res := runTerraform(ctx, dir, env, "apply", "-auto-approve", "-input=false",
		"-var=app_name="+appName, "-var=app_description=partial failure test")
	if res.Code != 1 {
		t.Fatalf("expected apply exit 1 (export failure), got %d\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "app-42") {
		t.Fatalf("error must name the created app id\nstderr:\n%s", res.Stderr)
	}
	if strings.Contains(res.Stdout+res.Stderr, "fake-token") {
		t.Fatalf("error output leaks the bearer token\nstdout:\n%s\nstderr:\n%s", res.Stdout, res.Stderr)
	}
	if exportHits == 0 {
		t.Fatal("expected at least one export call")
	}

	res = mustTF(t, ctx, dir, env, "state", "list")
	if !strings.Contains(res.Stdout, "dify_app.test") {
		t.Fatalf("created app missing from state after failed create\n%s", res.Stdout)
	}

	res = mustTF(t, ctx, dir, env, "show", "-json")
	var show struct {
		Values struct {
			RootModule struct {
				Resources []struct {
					Address string         `json:"address"`
					Values  map[string]any `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &show); err != nil {
		t.Fatalf("parsing terraform show: %v", err)
	}
	var id string
	for _, r := range show.Values.RootModule.Resources {
		if r.Address == "dify_app.test" {
			id, _ = r.Values["id"].(string)
		}
	}
	if id != "app-42" {
		t.Fatalf("partial state must persist remote id app-42, got %q", id)
	}
}
