resource "dify_model_credential" "backup" {
  provider_name = "langgenius/openai/openai"
  model         = "gpt-4o-mini"
  model_type    = "llm"
  name          = "backup-key"

  credentials = {
    api_key = var.backup_openai_api_key
  }
}

# Enable the model with a load-balancing pool: the first entry inherits the
# model's active credential, the second pins a specific credential.
resource "dify_model_settings" "gpt4o_mini" {
  provider_name          = "langgenius/openai/openai"
  model                  = "gpt-4o-mini"
  model_type             = "llm"
  config_from            = "predefined-model"
  enabled                = true
  load_balancing_enabled = true

  load_balancing_entries = [
    { enabled = true },
    { credential_id = dify_model_credential.backup.credential_id, enabled = true },
  ]
}

variable "backup_openai_api_key" {
  type      = string
  sensitive = true
}
