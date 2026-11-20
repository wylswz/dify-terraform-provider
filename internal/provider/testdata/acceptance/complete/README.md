# complete scenario

This scenario is intentionally **not** a copied Terraform graph: `TestAccComplete`
copies [`examples/complete`](../../../../../examples/complete) (the real example,
including its nested `dsls/` directory) into a temp dir and runs it verbatim, so
coverage cannot silently drift away from what the example demonstrates.

It exercises the full graph: marketplace plugin installs, model/tool provider
credentials, default models, load balancing, an API tool provider, an MCP
provider, published workflow apps, workflow tools, `dify_tools` data sources,
the orchestrator agent app, and an app API key.

Required inputs (mapped to `TF_VAR_*` inside the test):

| Env var           | `TF_VAR_*`        | Why                                  |
|-------------------|-------------------|--------------------------------------|
| `DIFY_HOST`       | `dify_server`     | Dify base URL                        |
| `DIFY_TOKEN`      | `dify_token`      | Personal Access Token                |
| `OPENAI_API_KEY`  | `openai_api_key`  | Model provider credential            |
| `TAVILY_API_KEY`  | `tavily_api_key`  | Tool provider credential             |
| `MCP_SERVER_URL`  | `mcp_server_url`  | Optional; defaults per the example   |

Gated behind `TF_ACC_COMPLETE=1` (see `make testacc-complete`); run it against
a dedicated, throwaway workspace only.
