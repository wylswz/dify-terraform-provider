// Copyright Dify Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPublishApp(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"version_id":"v-1","created_at":1700000000,"warning":null}`))
	})

	result, err := client.PublishApp(context.Background(), "app-1", "", "")
	if err != nil {
		t.Fatalf("PublishApp failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/openapi/v1/apps/app-1:publish" {
		t.Fatalf("expected POST /openapi/v1/apps/app-1:publish, got %s %s", gotMethod, gotPath)
	}
	if gotBody["marked_name"] != "" || gotBody["marked_comment"] != "" {
		t.Fatalf("unexpected publish body: %v", gotBody)
	}
	if result.VersionID != "v-1" {
		t.Fatalf("expected version_id v-1, got %q", result.VersionID)
	}
}

func TestServiceAPIAgentPathAndPatchBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"enabled":true,"base_url":"https://api.example.com","access_ready":true,"api_rpm":0,"api_rph":0}`))
	})

	result, err := client.SetServiceAPI(context.Background(), "app-1", true, true)
	if err != nil {
		t.Fatalf("SetServiceAPI failed: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/openapi/v1/apps/app-1/service-api/agent" {
		t.Fatalf("expected PATCH /apps/app-1/service-api/agent, got %s %s", gotMethod, gotPath)
	}
	if gotBody["enabled"] != true {
		t.Fatalf("expected enabled true, got %v", gotBody)
	}
	if !result.Enabled {
		t.Fatal("expected enabled response")
	}

	_, err = client.GetServiceAPI(context.Background(), "app-1", false)
	if err != nil {
		t.Fatalf("GetServiceAPI failed: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/openapi/v1/apps/app-1/service-api" {
		t.Fatalf("expected GET /apps/app-1/service-api, got %s %s", gotMethod, gotPath)
	}
}

func TestActivateModelProviderCredential(t *testing.T) {
	var gotMethod, gotPath string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"cred-1","name":"key","active":true}`))
	})

	result, err := client.ActivateModelProviderCredential(context.Background(), "langgenius/openai/openai", "cred-1")
	if err != nil {
		t.Fatalf("ActivateModelProviderCredential failed: %v", err)
	}
	want := "/openapi/v1/workspaces/ws-1/model-providers/langgenius/openai/openai/credentials/cred-1:activate"
	if gotMethod != http.MethodPost || gotPath != want {
		t.Fatalf("expected POST %s, got %s %s", want, gotMethod, gotPath)
	}
	if !result.Active {
		t.Fatal("expected active credential response")
	}
}

func TestModelScopedCredentialRoundTrip(t *testing.T) {
	var calls []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/models/credentials"):
			if body["model"] != "gpt-4o-mini" || body["model_type"] != "llm" || body["name"] != "backup" {
				t.Errorf("unexpected create body: %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"mc-1","name":"backup","active":false}`))
		case strings.HasSuffix(r.URL.Path, "/models/credentials/mc-1:activate"):
			if body["model"] != "gpt-4o-mini" || body["model_type"] != "llm" {
				t.Errorf("unexpected activate body: %v", body)
			}
			_, _ = w.Write([]byte(`{"id":"mc-1","name":"backup","active":true}`))
		case strings.HasSuffix(r.URL.Path, "/models/credentials/mc-1") && r.Method == http.MethodDelete:
			if r.URL.Query().Get("model") != "gpt-4o-mini" || r.URL.Query().Get("model_type") != "llm" {
				t.Errorf("expected model/model_type query params, got %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"result":"success"}`))
		case strings.HasSuffix(r.URL.Path, "/models/credentials/mc-1") && r.Method == http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"mc-1","name":"backup","active":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	res, err := client.CreateModelScopedCredential(context.Background(), "p/p/p", "gpt-4o-mini", "llm", map[string]any{"api_key": "sk"}, "backup")
	if err != nil || res.ID != "mc-1" {
		t.Fatalf("CreateModelScopedCredential: %v %v", res, err)
	}
	if _, err := client.UpdateModelScopedCredential(context.Background(), "p/p/p", "gpt-4o-mini", "llm", "mc-1", map[string]any{"api_key": "sk2"}, "backup"); err != nil {
		t.Fatalf("UpdateModelScopedCredential: %v", err)
	}
	res, err = client.ActivateModelScopedCredential(context.Background(), "p/p/p", "gpt-4o-mini", "llm", "mc-1")
	if err != nil || !res.Active {
		t.Fatalf("ActivateModelScopedCredential: %v %v", res, err)
	}
	if err := client.DeleteModelScopedCredential(context.Background(), "p/p/p", "gpt-4o-mini", "llm", "mc-1"); err != nil {
		t.Fatalf("DeleteModelScopedCredential: %v", err)
	}

	wantCalls := []string{
		"POST /openapi/v1/workspaces/ws-1/model-providers/p/p/p/models/credentials",
		"PATCH /openapi/v1/workspaces/ws-1/model-providers/p/p/p/models/credentials/mc-1",
		"POST /openapi/v1/workspaces/ws-1/model-providers/p/p/p/models/credentials/mc-1:activate",
		"DELETE /openapi/v1/workspaces/ws-1/model-providers/p/p/p/models/credentials/mc-1?model=gpt-4o-mini&model_type=llm",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("expected %d calls, got %v", len(wantCalls), calls)
	}
	for i, want := range wantCalls {
		if calls[i] != want {
			t.Fatalf("call %d: expected %s, got %s", i, want, calls[i])
		}
	}
}

func TestModelScopedCredentialNotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"credential_not_found","message":"nope","status":404}`))
	})

	err := client.DeleteModelScopedCredential(context.Background(), "p", "m", "llm", "missing")
	if !isNotFound(err) {
		t.Fatalf("expected isNotFound, got %v", err)
	}
}

func TestDefaultModelAndSettings(t *testing.T) {
	var calls []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case strings.Contains(r.URL.Path, "/default-models/llm") && r.Method == http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["provider"] != "langgenius/openai/openai" || body["model"] != "gpt-4o-mini" {
				t.Errorf("unexpected default-model body: %v", body)
			}
			_, _ = w.Write([]byte(`{"model_type":"llm","provider":"langgenius/openai/openai","model":"gpt-4o-mini"}`))
		case strings.Contains(r.URL.Path, "/default-models/llm"):
			_, _ = w.Write([]byte(`{"model_type":"llm","provider":"langgenius/openai/openai","model":"gpt-4o-mini"}`))
		case strings.Contains(r.URL.Path, "/models/settings") && r.Method == http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["config_from"] != "predefined-model" || body["enabled"] != true {
				t.Errorf("unexpected settings body: %v", body)
			}
			lb, _ := body["load_balancing"].(map[string]any)
			entries, _ := lb["entries"].([]any)
			if lb["enabled"] != true || len(entries) != 2 {
				t.Errorf("unexpected load_balancing body: %v", lb)
			}
			first, _ := entries[0].(map[string]any)
			if v, ok := first["credential_id"]; !ok || v != nil {
				t.Errorf("expected null credential_id for inherit entry, got %v", first)
			}
			second, _ := entries[1].(map[string]any)
			if second["credential_id"] != "cred-2" {
				t.Errorf("expected credential_id cred-2, got %v", second)
			}
			_, _ = w.Write([]byte(`{"model":"gpt-4o-mini","model_type":"llm","config_from":"predefined-model","enabled":true,"load_balancing":{"enabled":true,"entries":[{"id":"e1","name":"__inherit__","credential_id":null,"enabled":true,"inherit":true},{"id":"e2","name":"backup","credential_id":"cred-2","enabled":true,"inherit":false}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	def, err := client.SetDefaultModel(context.Background(), "llm", "langgenius/openai/openai", "gpt-4o-mini")
	if err != nil || def.Model == nil || *def.Model != "gpt-4o-mini" {
		t.Fatalf("SetDefaultModel: %v %v", def, err)
	}
	def, err = client.GetDefaultModel(context.Background(), "llm")
	if err != nil || def.Provider == nil || *def.Provider != "langgenius/openai/openai" {
		t.Fatalf("GetDefaultModel: %v %v", def, err)
	}

	settings, err := client.SetModelSettings(context.Background(), "langgenius/openai/openai", "gpt-4o-mini", "llm", "predefined-model", true, true, []LoadBalancingEntryRequest{
		{CredentialID: nil, Enabled: true},
		{CredentialID: strptr("cred-2"), Enabled: true},
	})
	if err != nil {
		t.Fatalf("SetModelSettings: %v", err)
	}
	if len(settings.LoadBalancing.Entries) != 2 || !settings.LoadBalancing.Entries[0].Inherit {
		t.Fatalf("unexpected settings response: %+v", settings.LoadBalancing)
	}
}

func strptr(s string) *string { return &s }

func TestAPIToolProviderRename(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"name":"weather-v2","icon":{"content":"⛅","background":"#252525"},"schema_type":"openapi","schema":"{}","credentials":{"auth_type":"none"},"privacy_policy":null,"custom_disclaimer":null,"labels":[],"tools":["forecast"],"description":null}`))
	})

	result, err := client.UpdateAPIToolProvider(context.Background(), "weather", apiToolProviderRequest{
		Name:        "weather-v2",
		Icon:        map[string]string{"content": "⛅", "background": "#252525"},
		SchemaType:  "openapi",
		Schema:      "{}",
		Credentials: map[string]any{"auth_type": "none"},
	})
	if err != nil {
		t.Fatalf("UpdateAPIToolProvider failed: %v", err)
	}
	want := "/openapi/v1/workspaces/ws-1/api-tool-providers/weather"
	if gotMethod != http.MethodPut || gotPath != want {
		t.Fatalf("expected PUT %s, got %s %s", want, gotMethod, gotPath)
	}
	if gotBody["name"] != "weather-v2" {
		t.Fatalf("expected rename via body name, got %v", gotBody["name"])
	}
	if result.Name != "weather-v2" || len(result.Tools) != 1 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestAPIToolProviderNotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"provider_not_found","message":"missing","status":404}`))
	})

	_, err := client.GetAPIToolProvider(context.Background(), "missing")
	if !isNotFound(err) {
		t.Fatalf("expected isNotFound, got %v", err)
	}
}

func TestMCPProviderRoundTripAndRefresh(t *testing.T) {
	var calls []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/mcp-providers") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["server_url"] != "https://mcp.example.com/mcp" || body["icon_type"] != "emoji" {
				t.Errorf("unexpected create body: %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"mcp-1","name":"deepwiki","icon":"🌐","server_url":"https://mcp.example.com/*****","server_identifier":"deepwiki","masked_headers":null,"authentication":null,"is_team_authorization":false,"is_dynamic_registration":true,"configuration":{"timeout":30,"sse_read_timeout":300},"identity_mode":"off","updated_at":1700000000,"tools":[]}`))
		case strings.HasSuffix(r.URL.Path, "/mcp-providers/mcp-1:refresh"):
			_, _ = w.Write([]byte(`{"id":"mcp-1","name":"deepwiki","icon":"🌐","server_url":"https://mcp.example.com/*****","server_identifier":"deepwiki","masked_headers":null,"authentication":null,"is_team_authorization":false,"is_dynamic_registration":true,"configuration":{"timeout":30,"sse_read_timeout":300},"identity_mode":"off","updated_at":1700000001,"tools":["ask_question"]}`))
		case strings.HasSuffix(r.URL.Path, "/mcp-providers/mcp-1") && r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"result":"success"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	result, err := client.CreateMCPProvider(context.Background(), mcpProviderRequest{
		ServerURL:        "https://mcp.example.com/mcp",
		Name:             "deepwiki",
		Icon:             "🌐",
		IconType:         "emoji",
		ServerIdentifier: "deepwiki",
	})
	if err != nil || result.ID != "mcp-1" {
		t.Fatalf("CreateMCPProvider: %v %v", result, err)
	}
	result, err = client.RefreshMCPProvider(context.Background(), "mcp-1")
	if err != nil || len(result.Tools) != 1 || result.Tools[0] != "ask_question" {
		t.Fatalf("RefreshMCPProvider: %v %v", result, err)
	}
	if err := client.DeleteMCPProvider(context.Background(), "mcp-1"); err != nil {
		t.Fatalf("DeleteMCPProvider: %v", err)
	}

	want := []string{
		"POST /openapi/v1/workspaces/ws-1/mcp-providers",
		"POST /openapi/v1/workspaces/ws-1/mcp-providers/mcp-1:refresh",
		"DELETE /openapi/v1/workspaces/ws-1/mcp-providers/mcp-1",
	}
	if len(calls) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
	for i, w := range want {
		if calls[i] != w {
			t.Fatalf("call %d: expected %s, got %s", i, w, calls[i])
		}
	}
}

func TestWorkflowToolCreateAndUpdate(t *testing.T) {
	var gotCreate, gotUpdate map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/workflow-tools") && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&gotCreate)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"web_research","label":"Web Research","workflow_tool_id":"wt-1","workflow_app_id":"app-1","icon":{"content":"📝","background":"#252525"},"description":"d","parameters":[{"name":"query","description":"q","form":"llm"}],"output_schema":{},"synced":true,"privacy_policy":null}`))
		case strings.HasSuffix(r.URL.Path, "/workflow-tools/wt-1") && r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotUpdate)
			_, _ = w.Write([]byte(`{"name":"web_research","label":"Web Research","workflow_tool_id":"wt-1","workflow_app_id":"app-1","icon":{"content":"📝","background":"#252525"},"description":"d2","parameters":[],"output_schema":{},"synced":true,"privacy_policy":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	result, err := client.CreateWorkflowTool(context.Background(), workflowToolRequest{
		WorkflowAppID: "app-1",
		Name:          "web_research",
		Label:         "Web Research",
		Icon:          map[string]string{"content": "📝", "background": "#252525"},
		Description:   "d",
		Parameters:    []WorkflowToolParameter{{Name: "query", Description: "q", Form: "llm"}},
	})
	if err != nil || result.WorkflowToolID != "wt-1" {
		t.Fatalf("CreateWorkflowTool: %v %v", result, err)
	}
	if gotCreate["workflow_app_id"] != "app-1" {
		t.Fatalf("expected workflow_app_id in create body, got %v", gotCreate)
	}

	result, err = client.UpdateWorkflowTool(context.Background(), "wt-1", workflowToolRequest{
		Name:        "web_research",
		Label:       "Web Research",
		Icon:        map[string]string{"content": "📝", "background": "#252525"},
		Description: "d2",
		Parameters:  []WorkflowToolParameter{},
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowTool: %v", err)
	}
	if _, has := gotUpdate["workflow_app_id"]; has {
		t.Fatalf("update body must not carry workflow_app_id, got %v", gotUpdate)
	}
	if result.Description != "d2" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestWorkflowToolNotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"provider_not_found","message":"missing","status":404}`))
	})

	err := client.DeleteWorkflowTool(context.Background(), "missing")
	if !isNotFound(err) {
		t.Fatalf("expected isNotFound, got %v", err)
	}
}

func TestListToolsPaginatesAndCarriesNodeData(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/openapi/v1/workspaces/ws-1/tools" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("provider") != "langgenius/tavily/tavily" {
			t.Errorf("expected provider filter, got %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"page":1,"limit":1,"total":2,"has_more":true,"data":[{"provider":"langgenius/tavily/tavily","provider_type":"builtin","provider_label":"Tavily","configured":true,"name":"tavily_search","label":"Tavily Search","description":"search","parameters":[],"node_data":{"type":"tool","provider_id":"langgenius/tavily/tavily","provider_name":"langgenius/tavily/tavily","plugin_id":"langgenius/tavily","plugin_unique_identifier":"langgenius/tavily:0.1.7@abc","tool_name":"tavily_search","tool_label":"Tavily Search","tool_node_version":"2","tool_parameters":{},"tool_configurations":{}}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"page":2,"limit":1,"total":2,"has_more":false,"data":[{"provider":"langgenius/tavily/tavily","provider_type":"builtin","provider_label":"Tavily","configured":true,"name":"tavily_extract","label":"Tavily Extract","description":null,"parameters":[],"node_data":{}}]}`))
	})

	tools, err := client.ListTools(context.Background(), "langgenius/tavily/tavily", "")
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools across pages, got %d", len(tools))
	}
	var nodeData map[string]any
	if err := json.Unmarshal(tools[0].NodeData, &nodeData); err != nil {
		t.Fatalf("node_data is not JSON: %v", err)
	}
	if nodeData["provider_id"] != "langgenius/tavily/tavily" || nodeData["tool_name"] != "tavily_search" {
		t.Fatalf("unexpected node_data: %v", nodeData)
	}
}
