# List models in the workspace, optionally filtered by provider and type.
data "dify_models" "openai_llms" {
  provider_name = "langgenius/openai/openai"
  model_type    = "llm"
}
