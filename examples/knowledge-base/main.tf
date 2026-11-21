# Terraform

terraform {
  required_providers {
    dify = {
      source  = "wylswz/dify"
      version = "0.0.1"
    }
  }
}

provider "dify" {
  host         = var.dify_server
  token        = var.dify_token
  workspace_id = var.dify_workspace_id
}

# ---------------------------------------------------------------------------
# Knowledge base
# ---------------------------------------------------------------------------

# indexing_technique = "economy" uses keyword indexing and needs no embedding
# model. Switch to "high_quality" once the workspace has a default embedding
# model configured.
resource "dify_dataset" "kb" {
  name               = var.dataset_name
  description        = "Product documentation knowledge base"
  indexing_technique = var.indexing_technique
  permission         = "all_team_members"
}

# Every file under docs/ becomes a knowledge base document. Edit the files
# and re-apply to update the corpus like any other code-managed content.
locals {
  docs = fileset("${path.module}/docs", "*.md")
}

resource "dify_dataset_document" "docs" {
  for_each      = local.docs
  dataset_id    = dify_dataset.kb.id
  name          = each.value
  text          = file("${path.module}/docs/${each.value}")
  doc_form      = "text_model"
  doc_language  = "English"
  indexing_wait = false
}

resource "dify_dataset_document_indexing_wait" "wait" {
  for_each           = local.docs
  dataset_id         = dify_dataset_document.docs[each.key].dataset_id
  document_id        = dify_dataset_document.docs[each.key].id
  timeout            = var.indexing_timeout
  polling_interval   = 5

  depends_on = [dify_dataset_document.docs]
}

# ---------------------------------------------------------------------------
# Retrieval workflow
# ---------------------------------------------------------------------------

# The DSL is a template because the knowledge-retrieval node's dataset_ids
# can only be known after the dataset exists.
resource "dify_app" "retrieval" {
  dsl_yaml = templatefile("${path.module}/dsls/retrieval.yml.tftpl", {
    dataset_id = dify_dataset.kb.id
  })
  name        = "Knowledge Base Search"
  description = "Recalls relevant passages from the managed knowledge base"
  publish     = true
}
