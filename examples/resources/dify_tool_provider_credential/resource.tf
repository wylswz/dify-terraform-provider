# Install the tool plugin first.
resource "dify_plugin_install" "tavily" {
  plugin_unique_identifier = "langgenius/tavily:0.1.7@5fce9cf01fecda9ad92e5125397d2bb5497429baed276c7f14f033e7debd0abe"
}

# Then configure credentials for the tool provider.
resource "dify_tool_provider_credential" "tavily" {
  provider_name = "langgenius/tavily/tavily"
  name          = "production-key"

  credentials = {
    tavily_api_key = var.tavily_api_key
  }

  depends_on = [dify_plugin_install.tavily]
}

variable "tavily_api_key" {
  type      = string
  sensitive = true
}
