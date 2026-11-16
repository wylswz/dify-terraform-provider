> This is a demo project
# Dify Terraform Provider

A Terraform provider for managing [Dify](https://dify.ai) applications, model and tool provider credentials, plugin installs, and app API keys as infrastructure-as-code.

The provider talks to Dify's user-scoped OpenAPI (`/openapi/v1`) and authenticates with a **Personal Access Token (PAT)** — no admin API key or special backend branch is required.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://go.dev/dl/) >= 1.22 (to build the provider)
- A Dify instance exposing the `/openapi/v1` API

## Personal Access Token

Create a PAT in the Dify console: **account settings → Personal Access Tokens**. The token acts as your user account, so it must have access to the workspace you manage — including workspace-scoped permissions for the operations you run (app import/export, credential management, plugin install, and so on). Treat it like a password: it is stored in Terraform state.

## Quick Start

```hcl
terraform {
  required_providers {
    dify = {
      source  = "registry.terraform.io/dify/dify"
      version = "0.1.0"
    }
  }
}

provider "dify" {
  host         = "https://dify.example.com"
  token        = var.dify_token
  workspace_id = var.dify_workspace_id
}
```

## Provider Configuration

| Attribute | Environment Variable | Description | Required |
|---|---|---|---|
| `host` | `DIFY_HOST` | Dify base URL; the provider calls `<host>/openapi/v1` | No (defaults to `http://localhost`) |
| `token` | `DIFY_TOKEN` | Personal Access Token (`Authorization: Bearer`) | Yes |
| `workspace_id` | `DIFY_WORKSPACE_ID` | Workspace ID for resource operations | Yes |

The server rate-limits each token on `/openapi/v1` (`OPENAPI_RATE_LIMIT_PER_TOKEN`, default 60 requests/minute). The provider automatically backs off and retries on HTTP 429 using the response's `Retry-After` header.

![Dependency Graph](img/plan.svg)

## Resources

### `dify_app`

Manages a Dify application via DSL YAML content. The app lifecycle (create, update, delete) is driven by importing the DSL through the workspace OpenAPI.

```hcl
resource "dify_app" "my_workflow" {
  dsl_yaml    = file("my-workflow.yaml")
  name        = "My Workflow"
  description = "A workflow managed by Terraform"
}
```

| Attribute | Type | Description |
|---|---|---|
| `dsl_yaml` | String (required) | DSL YAML content defining the app |
| `name` | String (optional) | Override app name from DSL |
| `description` | String (optional) | Override app description from DSL |
| `id` | String (computed) | App ID |
| `mode` | String (computed) | App mode (`chat`, `workflow`, `agent-chat`, etc.), read from the exported DSL |
| `exported_dsl_yaml` | String (computed) | DSL YAML exported from the live app |
| `draft_hash` | String (computed) | Draft hash returned by the export; sent back on overwrite imports |

**Replace-on-update caveat**: the OpenAPI only overwrites `workflow` and `advanced-chat` apps in place. For apps of any other mode (`chat`, `agent-chat`, `completion`, `agent`), changing `dsl_yaml`, `name`, or `description` **replaces** the app (delete + create) — plan accordingly, since replacement generates a new app ID.

The resource can be imported by app ID: `terraform import dify_app.my_app <app_id>`.

### `dify_model_provider_credential`

Manages model provider credentials (e.g. an OpenAI API key). `provider_name` is the provider path, e.g. `langgenius/openai/openai`.

```hcl
resource "dify_model_provider_credential" "openai" {
  provider_name = "langgenius/openai/openai"
  name          = "production-key"
  credentials = {
    openai_api_key = "sk-..."
  }
}
```

| Attribute | Type | Description |
|---|---|---|
| `provider_name` | String (required) | Model provider id (forces replacement if changed) |
| `credentials` | Map (required, sensitive) | Credential key-value pairs; write-only — the API never returns secret values, so state keeps the configured values |
| `name` | String (optional) | Human-readable name |
| `credential_id` | String (computed) | Credential ID |
| `active` | Bool (computed) | Whether this is the provider's active credential |

### `dify_tool_provider_credential`

Manages builtin tool provider credentials (e.g. a Tavily API key). Same shape as the model credential resource; `name` is limited to 30 characters by the API.

```hcl
# Install the plugin first
resource "dify_plugin_install" "tavily" {
  plugin_unique_identifier = "langgenius/tavily:0.1.7@5fce9cf01fecda9ad92e5125397d2bb5497429baed276c7f14f033e7debd0abe"
}

# Then configure credentials
resource "dify_tool_provider_credential" "tavily" {
  provider_name = "langgenius/tavily/tavily"
  name          = "production-key"
  credentials = {
    tavily_api_key = "tvly-..."
  }

  depends_on = [dify_plugin_install.tavily]
}
```

| Attribute | Type | Description |
|---|---|---|
| `provider_name` | String (required) | Tool provider id — for plugins use the full path `langgenius/tavily/tavily` |
| `credentials` | Map (required, sensitive) | Credential key-value pairs; write-only |
| `name` | String (optional) | Human-readable name (max 30 chars) |
| `credential_id` | String (computed) | Credential ID |
| `active` | Bool (computed) | Whether this is the provider's default credential |

### `dify_plugin_install`

Manages a plugin installation from the marketplace. Installation is asynchronous; the resource polls the install task until it finishes.

```hcl
resource "dify_plugin_install" "openai" {
  plugin_unique_identifier = "langgenius/openai:0.3.8@592c8252795b5f75807de2d609a03196ed02596b409f7642b4a07548c7ff57ef"
}
```

| Attribute | Type | Description |
|---|---|---|
| `plugin_unique_identifier` | String (required) | Versioned marketplace identifier (`name:version@sha256`) |
| `plugin_id` | String (computed) | Plugin ID (`langgenius/openai`) |
| `version` | String (computed) | Installed version |
| `category` | String (computed) | Plugin category (`model`, `tool`, …) |

### `dify_app_api_key`

Manages an API key for a Dify application.

```hcl
resource "dify_app_api_key" "key" {
  app_id = dify_app.my_workflow.id
}
```

| Attribute | Type | Description |
|---|---|---|
| `app_id` | String (required) | App ID (forces replacement if changed) |
| `key_id` | String (computed) | API key ID |
| `token` | String (computed, sensitive) | API key token |
| `created_at` | String (computed) | Creation timestamp (RFC3339) |

## Data Sources

### `dify_app`

Reads a Dify application by `id` or exact `name` (exactly one required).

```hcl
data "dify_app" "existing" {
  id = "abc-123-def"
}
```

Returns `id`, `name`, `mode`, `description`, and `updated_at`.

### `dify_plugins`

Lists all plugins installed in the workspace.

```hcl
data "dify_plugins" "all" {}
```

Each entry exposes `plugin_id`, `identifier`, `version`, `category`, and `source`.

### `dify_models`

Lists models available in the workspace, with optional `provider_name` and `model_type` filters.

```hcl
data "dify_models" "openai_llms" {
  provider_name = "langgenius/openai/openai"
  model_type    = "llm"
}
```

Each entry exposes `provider`, `model`, `model_type`, and `status`.

## Datasets

Dataset and dataset-document management is **not yet supported** by this provider.

## Developing the Provider

```bash
# Build
go build ./...

# Install locally for development
make install

# Run tests
go test ./...
```

## DSL YAML

The `dify_app` resource uses Dify's DSL YAML format for defining applications. Export an existing app's DSL from the Dify console (or via `GET /openapi/v1/apps/<app_id>/dsl`), save it, and manage it in Terraform:

```hcl
resource "dify_app" "new_app" {
  dsl_yaml = file("exported-app.yaml")
  name     = "Cloned App"
}
```

After apply, `exported_dsl_yaml` holds the server-normalized DSL — useful for reviewing drift between your `dsl_yaml` and what Dify actually stored.

## License

MPL-2.0
