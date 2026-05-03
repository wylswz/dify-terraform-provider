terraform {
  required_providers {
    dify = {
      source  = "langgenius/dify"
      version = "0.1.0"
    }
  }
}

provider "dify" {
  host         = "http://localhost"
  api_key      = var.dify_admin_api_key
  workspace_id = var.dify_workspace_id
}

variable "dify_admin_api_key" {
  type      = string
  sensitive = true
}

variable "openai_api_key" {
  type      = string
  sensitive = true
  default = ""
}

variable "tavily_api_key" {
  type      = string
  sensitive = true
  default = ""
}

variable "dify_workspace_id" {
  type = string
}


# ---------------------------------------------------------------------------
# Step 1: Install the OpenAI model plugin from marketplace
# The resource polls until the plugin is ready before proceeding.
# ---------------------------------------------------------------------------
resource "dify_plugin_install" "openai" {
  plugin_unique_identifier = "langgenius/openai:0.3.8@592c8252795b5f75807de2d609a03196ed02596b409f7642b4a07548c7ff57ef"
  source                   = "marketplace"
}

resource "dify_plugin_install" "tavily" {
  plugin_unique_identifier = "langgenius/tavily:0.1.7@5fce9cf01fecda9ad92e5125397d2bb5497429baed276c7f14f033e7debd0abe"
  source                   = "marketplace"
}

# ---------------------------------------------------------------------------
# Step 2: Configure OpenAI model provider credentials
# Depends on the plugin being installed first.
# ---------------------------------------------------------------------------
resource "dify_model_provider_credential" "openai" {
  provider_name = "openai"
  name          = "production-key"
  credentials = {
    openai_api_key = var.openai_api_key
  }

  depends_on = [dify_plugin_install.openai]
}

# ---------------------------------------------------------------------------
# Step 2b: Configure Tavily tool provider credentials
# Tavily is a tool plugin, so it uses a separate resource type.
# Note: provider_name must use the full format langgenius/tavily/tavily
# ---------------------------------------------------------------------------
resource "dify_tool_provider_credential" "tavily" {
  provider_name = "langgenius/tavily/tavily"
  name          = "production-key"
  credentials = {
    tavily_api_key = var.tavily_api_key
  }

  depends_on = [dify_plugin_install.tavily]
}

# ---------------------------------------------------------------------------
# Step 3: Create a workflow app from DSL YAML
# Depends on credentials being configured so the app can reference models.
# ---------------------------------------------------------------------------
resource "dify_app" "my_workflow" {
  creator_email = "yunlu.wen@dify.ai"
  dsl_yaml      = file("dsls/test.yml")
  name          = "My Workflow"
  description   = "A workflow managed by Terraform"

  depends_on = [
    dify_model_provider_credential.openai,
    dify_tool_provider_credential.tavily
  ]
}

# ---------------------------------------------------------------------------
# Step 4: Create an API key for the workflow app
# ---------------------------------------------------------------------------
resource "dify_app_api_key" "workflow_key" {
  app_id = dify_app.my_workflow.id
}

# List model providers
data "dify_model_providers" "all" {}

# List installed plugins
data "dify_plugins" "all" {}

output "workflow_app_id" {
  value = dify_app.my_workflow.id
}

output "workflow_api_key" {
  value     = dify_app_api_key.workflow_key.token
  sensitive = true
}
