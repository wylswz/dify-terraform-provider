# Wait for a document's indexing to complete.
# The document is created by a separate dify_dataset_document resource with
# indexing_wait = false.
resource "dify_dataset_document_indexing_wait" "wait" {
  dataset_id       = "<dataset_id>"
  document_id      = "<document_id>"
  timeout          = 600
  polling_interval = 5
}