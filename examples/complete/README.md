# Complete Example

An end-to-end workspace: model credentials with load balancing, default models,
tool providers (built-in, OpenAPI-schema, MCP), three published workflow apps
exposed as tools, and an agent-chat orchestrator that calls them all.

## Layout

- `main.tf` — all resources, wired by attribute references (the Terraform
  graph does the ordering; `depends_on` is used only where no data reference exists)
- `variables.tf` / `outputs.tf`
- `dsls/*.yml` — static DSL documents
- `dsls/*.yml.tftpl` — DSL templates rendered with `templatefile()` where
  server-assigned ids (workflow tool ids, MCP provider id, API tool provider
  name) must be injected
- `dsls/open_meteo_schema.json` — the OpenAPI 3 schema the `open_meteo`
  API tool provider serves

## Usage

```bash
export DIFY_SERVER=https://dify.example.com
export DIFY_TOKEN=...        # Personal Access Token
export OPENAI_API_KEY=...
export TAVILY_API_KEY=...
export DIFY_WORKSPACE_ID=...  # optional; empty = the token owner's default workspace

terraform init
terraform apply \
  -var dify_token=$DIFY_TOKEN \
  -var dify_server=$DIFY_SERVER \
  -var openai_api_key=$OPENAI_API_KEY \
  -var tavily_api_key=$TAVILY_API_KEY \
  -var dify_workspace_id=$DIFY_WORKSPACE_ID
```

`dify_workspace_id` may be empty or omitted (it then defaults to the token
owner's default workspace). Other optional vars: `openai_api_key_backup`
(defaults to `openai_api_key`) and `mcp_server_url` (defaults to `https://mcp.deepwiki.com/mcp`).

### Running against a locally built provider

`wylswz/dify` is not published, so point Terraform at a local `make
install` build with a `dev_overrides` CLI config:

```hcl
provider_installation {
  dev_overrides {
    "wylswz/dify" = "<path to the built provider dir, e.g. ~/.terraform.d/plugins/registry.terraform.io/wylswz/dify/0.1.0/darwin_arm64>"
  }
  direct {}
}
```

```bash
TF_CLI_CONFIG_FILE=/tmp/dify-dev.tfrc terraform apply ...
```

With `dev_overrides`, `terraform init` is unnecessary and may error — skip it.

## DSL notes

DSL documents use `kind: app` / `version: 0.7.0`
(`CURRENT_APP_DSL_VERSION` in `api/services/app_dsl_service.py`). Tool-node
shapes follow `ToolNodeTemplate` / `ToolRow.node_data` in
`api/controllers/openapi/_models.py`.

Tool ids and provider fields are not guessed: `data "dify_tools"` reads the
server's own `node_data` template for each tool (workflow tools filtered by
`provider_name` = workflow tool id; MCP by provider id; open_meteo by `query`
since API providers carry a server DB id) and the DSLs inject
`provider_id`/`provider_name`/`provider_type`/`tool_name`/`tool_label` from it.

Fields not confirmed against a live instance — verify on first apply:

- **Agent-mode tool entries**: the `settings`, `parameters`, `extra`, and
  `tool_description` sub-fields mirror the console's stored shape; only
  `provider_id`, `provider_type`, `tool_name`, and `enabled` are validated by
  the server's agent config manager — the rest are best-effort UI fields.
- **Tool `form` values** on `dify_workflow_tool.parameters`: `llm` is the
  default; `form`/`schema` exist per `ToolParameterRow` but are unused here.
- The orchestrator is `agent-chat`, which has no draft — `publish` is omitted
  because the server only publishes `workflow`/`advanced-chat` apps; the app
  is live as soon as the DSL import lands.
