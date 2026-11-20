# Provider-level credential, e.g. an OpenAI API key shared by all models
# of the provider.
resource "dify_model_provider_credential" "openai" {
  provider_name = "langgenius/openai/openai"
  name          = "production-key"
  make_active   = true

  credentials = {
    openai_api_key = var.openai_api_key
  }
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}
