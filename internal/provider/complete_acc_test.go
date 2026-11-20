// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAccComplete runs the real examples/complete graph verbatim — not a
// copied rewrite — so coverage tracks the example. It needs the external
// credentials the example consumes and is therefore gated behind
// TF_ACC_COMPLETE=1 on top of TF_ACC=1 (see `make testacc-complete`).
func TestAccComplete(t *testing.T) {
	if os.Getenv("TF_ACC_COMPLETE") != "1" {
		t.Skip("set TF_ACC_COMPLETE=1 to exercise examples/complete")
	}
	if os.Getenv("TF_ACC") != "1" {
		t.Fatal("TF_ACC_COMPLETE requires TF_ACC=1")
	}
	host, token := accEnv(t)

	openaiKey := os.Getenv("OPENAI_API_KEY")
	tavilyKey := os.Getenv("TAVILY_API_KEY")
	if openaiKey == "" || tavilyKey == "" {
		t.Fatal("TF_ACC_COMPLETE=1 requires OPENAI_API_KEY and TAVILY_API_KEY")
	}

	cliConfig, scratch := buildProviderMirror(t)

	// Map credentials into the example's TF_VAR_* inputs; DIFY_WORKSPACE_ID
	// stays unset so the provider resolves the token's default workspace.
	env := append(terraformEnv(cliConfig),
		"TF_VAR_dify_server="+host,
		"TF_VAR_dify_token="+token,
		"TF_VAR_openai_api_key="+openaiKey,
		"TF_VAR_tavily_api_key="+tavilyKey,
	)
	if mcpURL := os.Getenv("MCP_SERVER_URL"); mcpURL != "" {
		env = append(env, "TF_VAR_mcp_server_url="+mcpURL)
	}

	ctx := context.Background()

	// Copy only the tracked example inputs — never stray .tfvars, state,
	// lockfiles, or CLI config that may sit in a working checkout.
	exampleDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "complete"))
	if err != nil {
		t.Fatalf("resolving examples/complete: %v", err)
	}
	dir := filepath.Join(scratch, "complete")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"main.tf", "variables.tf", "outputs.tf"} {
		data, err := os.ReadFile(filepath.Join(exampleDir, f))
		if err != nil {
			t.Fatalf("reading examples/complete/%s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
			t.Fatalf("copying %s: %v", f, err)
		}
	}
	if err := os.CopyFS(filepath.Join(dir, "dsls"), os.DirFS(filepath.Join(exampleDir, "dsls"))); err != nil {
		t.Fatalf("copying examples/complete/dsls: %v", err)
	}

	mustTF(t, ctx, dir, env, "init", "-input=false")

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		//nolint:errcheck // best-effort cleanup; failure is reported by the run.
		runTerraform(cleanCtx, dir, env, "destroy", "-auto-approve", "-input=false")
	})

	mustTF(t, ctx, dir, env, "apply", "-auto-approve", "-input=false")

	for _, key := range []string{
		"research_app_id",
		"translate_app_id",
		"weather_brief_app_id",
		"orchestrator_app_id",
		"web_research_tool_id",
		"translate_text_tool_id",
		"weather_brief_tool_id",
		"deepwiki_mcp_provider_id",
	} {
		if v := tfOutput(t, ctx, dir, env, key); v == "" {
			t.Fatalf("output %q is empty after apply", key)
		}
	}

	res := runTerraform(ctx, dir, env, "plan", "-detailed-exitcode", "-input=false")
	if res.Code != 0 {
		t.Fatalf("expected empty plan after apply (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}

	mustTF(t, ctx, dir, env, "destroy", "-auto-approve", "-input=false")
	fmt.Println("examples/complete destroyed cleanly")
}
