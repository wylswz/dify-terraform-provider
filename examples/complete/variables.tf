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

variable "openai_api_key" {
  type        = string
  sensitive   = true
  description = "OpenAI API key for the primary model provider credential"
}

variable "openai_api_key_backup" {
  type        = string
  sensitive   = true
  default     = null
  description = "OpenAI API key for the backup credential (defaults to the primary key)"
}

variable "tavily_api_key" {
  type        = string
  sensitive   = true
  description = "Tavily API key"
}

variable "mcp_server_url" {
  type        = string
  sensitive   = true
  default     = "https://mcp.deepwiki.com/mcp"
  description = "URL of the MCP server to register"
}
