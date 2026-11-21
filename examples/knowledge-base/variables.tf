variable "dify_server" {
  type        = string
  description = "Dify base URL, e.g. https://dify.example.com"
}

variable "dify_token" {
  type        = string
  sensitive   = true
  description = "Dify Personal Access Token (console → account settings → Personal Access Tokens)"
}

variable "dify_workspace_id" {
  type        = string
  default     = null
  description = "Defaults to the token owner's default workspace"
}

variable "dataset_name" {
  type        = string
  default     = "product-docs"
  description = "Name of the knowledge base to create"
}

variable "indexing_technique" {
  type        = string
  default     = "economy"
  description = "Dataset indexing technique: economy (keyword, no embedding model needed) or high_quality (requires a default embedding model in the workspace)"
}

variable "indexing_timeout" {
  type        = number
  default     = 600
  description = "Seconds to wait for each document's indexing to complete"
}
