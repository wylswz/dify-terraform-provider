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

// TestAccDatasetLifecycle exercises dify_dataset, dify_dataset_document and
// dify_dataset_document_indexing_wait against a live Dify instance.
// Documents are sourced from local files (.md and .txt) to mirror a
// code-repository workflow. Gated behind TF_ACC=1.
func TestAccDatasetLifecycle(t *testing.T) {
	host, token := accEnv(t)

	cliConfig, scratch := buildProviderMirror(t)
	env := terraformEnv(cliConfig)
	ctx := context.Background()

	datasetName := fmt.Sprintf("tfacc-ds-%d", time.Now().UnixNano())
	varArgs := []string{"-var=dataset_name=" + datasetName}

	dir := filepath.Join(scratch, "dataset_lifecycle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyAccScenario(t, "dataset_lifecycle", dir)

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		//nolint:errcheck // best-effort cleanup; failure is reported by the run.
		runTerraform(cleanCtx, dir, env, append([]string{"destroy", "-auto-approve", "-input=false"}, varArgs...)...)
	})

	mustTF(t, ctx, dir, env, "init", "-input=false")

	mustTF(t, ctx, dir, env, append([]string{"apply", "-auto-approve", "-input=false"}, varArgs...)...)

	datasetID := tfOutput(t, ctx, dir, env, "dataset_id")
	if datasetID == "" {
		t.Fatal("dataset_id output is empty after apply")
	}

	// Both local file formats (.md and .txt) must be imported as documents.
	count := tfOutput(t, ctx, dir, env, "doc_count")
	if count != "2" {
		t.Fatalf("expected 2 documents from local files, got %q", count)
	}

	docIDs := tfOutputJSON(t, ctx, dir, env, "doc_ids")
	if docIDs == "" {
		t.Fatal("doc_ids output is empty after apply")
	}

	// All documents must finish indexing.
	statuses := tfOutputJSON(t, ctx, dir, env, "doc_indexing_statuses")
	for _, want := range []string{"available", "available"} {
		if !containsStatus(statuses, want) {
			t.Fatalf("expected indexing_status %q among %q", want, statuses)
		}
	}

	// No-op plan: remote matches configuration.
	res := runTerraform(ctx, dir, env, append([]string{"plan", "-detailed-exitcode", "-input=false"}, varArgs...)...)
	if res.Code != 0 {
		t.Fatalf("expected empty plan after apply (exit %d)\nstdout:\n%s\nstderr:\n%s",
			res.Code, res.Stdout, res.Stderr)
	}

	// Confirm the dataset is remotely reachable.
	client := NewDifyClient(host, token, "")
	workspaceID, err := resolveWorkspaceID(ctx, client, "")
	if err != nil {
		t.Fatalf("resolving workspace: %v", err)
	}
	client.WorkspaceID = workspaceID
	_, err = client.GetDataset(ctx, datasetID)
	if err != nil {
		t.Fatalf("dataset %q not reachable remotely: %v", datasetID, err)
	}

	// The retrieval workflow must return the indexed document content.
	appID := tfOutput(t, ctx, dir, env, "app_id")
	if appID == "" {
		t.Fatal("app_id output is empty after apply")
	}
	outputs, err := client.RunWorkflow(ctx, appID, WorkflowRunRequest{
		Inputs: map[string]any{"query": "acceptance test document"},
	})
	if err != nil {
		t.Fatalf("running retrieval workflow %q: %v", appID, err)
	}
	if !containsSubstring(string(outputs), "acceptance test document") {
		t.Fatalf("workflow outputs missing retrieved content: %s", string(outputs))
	}

	// Destroy, then confirm the dataset is gone remotely.
	destroyArgs := append([]string{"destroy", "-auto-approve", "-input=false"}, varArgs...)
	mustTF(t, ctx, dir, env, destroyArgs...)

	if _, err := client.GetDataset(ctx, datasetID); err == nil {
		t.Fatalf("dataset %q still exists after destroy", datasetID)
	} else if !isNotFound(err) {
		t.Fatalf("expected 404 after destroy, got: %v", err)
	}
}

// containsStatus reports whether the JSON-ish statuses string contains the
// exact quoted status token.
func containsStatus(statuses, want string) bool {
	return containsSubstring(statuses, `"`+want+`"`)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
