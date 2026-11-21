# Create a knowledge base (dataset).
resource "dify_dataset" "kb" {
  name                     = "My Knowledge Base"
  description              = "Documents for RAG retrieval."
  indexing_technique       = "high_quality"
  permission               = "all_team_members"
  provider_type            = "vendor"
  embedding_model          = "text-embedding-v3"
  embedding_model_provider = "jina"
}

variable "retrieval_model" {
  type    = string
  default = ""
}

# retrieval_model is passed as a JSON string when set.
locals {
  retrieval_model = var.retrieval_model != "" ? var.retrieval_model : jsonencode({
    reranking_model = {
      reranking_provider_name = "jina"
      reranking_model_name    = "jina-reranker-v1"
    }
    top_n = 3
  })
}

# Create a document from a local file in the dataset.
resource "dify_dataset_document" "doc" {
  dataset_id    = dify_dataset.kb.id
  name          = "Example Document"
  text          = file("${path.module}/document.md")
  doc_form      = "text_model"
  doc_language  = "English"
  indexing_wait = true
}

# Optionally wait explicitly when indexing_wait is disabled on the document.
resource "dify_dataset_document_indexing_wait" "wait" {
  dataset_id       = dify_dataset_document.doc.dataset_id
  document_id      = dify_dataset_document.doc.id
  timeout          = 600
  polling_interval = 5

  depends_on = [dify_dataset_document.doc]
}