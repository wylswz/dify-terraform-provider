terraform {
  required_providers {
    dify = {
      source  = "wylswz/dify"
      version = "0.0.1"
    }
  }
}

provider "dify" {}

variable "dataset_name" {
  type = string
}

# A knowledge base with keyword indexing: the scratch stack has no embedding
# model configured, so high_quality cannot resolve a default embedding model.
resource "dify_dataset" "test" {
  name               = var.dataset_name
  description        = "acceptance test dataset"
  indexing_technique = "economy"
  permission         = "all_team_members"
}

# All documents under docs/ are imported as knowledge base documents,
# supporting at least two file formats (.md and .txt).
locals {
  docs = fileset("${path.module}/docs", "*.{md,txt}")
}

resource "dify_dataset_document" "docs" {
  for_each      = local.docs
  dataset_id    = dify_dataset.test.id
  name          = each.value
  text          = file("${path.module}/docs/${each.value}")
  doc_form      = "text_model"
  doc_language  = "English"
  indexing_wait = false
}

resource "dify_dataset_document_indexing_wait" "wait" {
  for_each         = local.docs
  dataset_id       = dify_dataset_document.docs[each.key].dataset_id
  document_id      = dify_dataset_document.docs[each.key].id
  timeout          = 300
  polling_interval = 3
  depends_on       = [dify_dataset_document.docs]
}

# A workflow app whose knowledge-retrieval node searches the dataset above.
# The dataset id is only known at apply time, so the DSL is a template.
resource "dify_app" "retrieval" {
  dsl_yaml = templatefile("${path.module}/workflow.yml.tftpl", {
    dataset_id = dify_dataset.test.id
  })
  publish = true
}

output "dataset_id" {
  value = dify_dataset.test.id
}

output "app_id" {
  value = dify_app.retrieval.id
}

output "doc_count" {
  value = length(local.docs)
}

output "doc_ids" {
  value = [for k, v in dify_dataset_document.docs : v.id]
}

output "doc_indexing_statuses" {
  value = [for k, v in dify_dataset_document_indexing_wait.wait : v.indexing_status]
}