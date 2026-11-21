# Create a knowledge base (dataset).
resource "dify_dataset" "kb" {
  name               = "My Knowledge Base"
  description        = "Documents for RAG retrieval."
  indexing_technique = "high_quality"
  permission         = "all_team_members"
}

# Document content is sourced from a local file, so the knowledge base can be
# maintained like a code repository: edit `document.md` and run `terraform apply`.
resource "dify_dataset_document" "doc" {
  dataset_id    = dify_dataset.kb.id
  name          = "Example Document"
  text          = file("${path.module}/document.md")
  doc_form      = "text_model"
  doc_language  = "English"
  indexing_wait = false
}

# Wait for the document's indexing to complete with a custom timeout.
resource "dify_dataset_document_indexing_wait" "wait" {
  dataset_id       = dify_dataset_document.doc.dataset_id
  document_id      = dify_dataset_document.doc.id
  timeout          = 600
  polling_interval = 5

  depends_on = [dify_dataset_document.doc]
}