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

variable "app_description" {
  type    = string
  default = "acceptance test"
}

resource "dify_app" "test" {
  dsl_yaml            = file("${path.module}/workflow.yml")
  name                = var.app_name
  description         = var.app_description
  service_api_enabled = true
}

resource "dify_app_api_key" "test" {
  app_id = dify_app.test.id
}

output "app_id" {
  value = dify_app.test.id
}

output "app_mode" {
  value = dify_app.test.mode
}

output "app_name" {
  value = dify_app.test.name
}

output "service_api_enabled" {
  value = tostring(dify_app.test.service_api_enabled)
}
