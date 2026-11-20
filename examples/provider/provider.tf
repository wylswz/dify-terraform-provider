terraform {
  required_providers {
    dify = {
      source = "registry.terraform.io/dify/dify"
    }
  }
}

# All attributes can also be set via environment variables:
#   DIFY_HOST, DIFY_TOKEN, DIFY_WORKSPACE_ID
provider "dify" {
  host         = "https://dify.example.com"
  token        = var.dify_token
  workspace_id = var.dify_workspace_id
}

variable "dify_token" {
  type      = string
  sensitive = true
}

variable "dify_workspace_id" {
  type    = string
  default = null
}
