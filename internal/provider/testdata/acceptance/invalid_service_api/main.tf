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

resource "dify_app" "test" {
  dsl_yaml            = file("${path.module}/workflow.yml")
  name                = var.app_name
  service_api_enabled = false
}
