# Credential for a single model (custom-model credential).
resource "dify_model_credential" "gpt4o_mini" {
  provider_name = "langgenius/openai/openai"
  model         = "gpt-4o-mini"
  model_type    = "llm"
  name          = "per-model-key"
  make_active   = true

  credentials = {
    api_key = var.openai_api_key
  }
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}
