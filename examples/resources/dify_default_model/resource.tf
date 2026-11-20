# Set the workspace default model for a model type.
resource "dify_default_model" "llm" {
  model_type    = "llm"
  provider_name = "langgenius/openai/openai"
  model         = "gpt-4o-mini"
}
