terraform {
  required_providers {
    dify = {
      source  = "wylswz/dify"
      version = "0.0.1"
    }
  }
}

provider "dify" {}

variable "app_name" {
  type = string
}

# The workflow app must be published before it can back a workflow tool.
resource "dify_app" "test" {
  dsl_yaml = file("${path.module}/workflow.yml")
  name     = var.app_name
  publish  = true
}

resource "dify_workflow_tool" "test" {
  workflow_app_id = dify_app.test.id
  name            = "acc_workflow_tool"
  label           = "Acc Workflow Tool"
  description     = "Workflow tool created by the acceptance test"
  icon = {
    content    = "🔧"
    background = "#E4FBCC"
  }
  parameters = [
    {
      name        = "text"
      description = "Text input declared by the workflow DSL"
      form        = "llm"
    },
  ]
}

# Static OpenAPI document; no outbound call is ever made.
resource "dify_api_tool_provider" "test" {
  name = "acc_api_tools"
  icon = {
    content    = "🛠️"
    background = "#E0F2FE"
  }
  schema_type = "openapi"
  schema      = file("${path.module}/openapi.json")
  credentials = { auth_type = "none" }
}

data "dify_tools" "workflow" {
  provider_name = dify_workflow_tool.test.id
}

data "dify_tools" "api" {
  query = "acc_api_tools"

  # Without a reference to the provider the read would run at plan time,
  # before the API tool provider exists.
  depends_on = [dify_api_tool_provider.test]
}

output "workflow_tool_id" {
  value = dify_workflow_tool.test.id
}

output "workflow_tool_names" {
  value = join(",", [for t in data.dify_tools.workflow.tools : t.name])
}

output "api_tool_provider_name" {
  value = dify_api_tool_provider.test.name
}

output "api_tool_names" {
  value = join(",", [for t in data.dify_tools.api.tools : t.name])
}
