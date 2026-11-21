output "dataset_id" {
  value = dify_dataset.kb.id
}

output "document_ids" {
  value = { for k, v in dify_dataset_document.docs : k => v.id }
}

output "document_indexing_statuses" {
  value = { for k, v in dify_dataset_document_indexing_wait.wait : k => v.indexing_status }
}

output "retrieval_app_id" {
  value = dify_app.retrieval.id
}
