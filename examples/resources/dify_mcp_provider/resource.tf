# Register an MCP (Model Context Protocol) tool provider.
resource "dify_mcp_provider" "deepwiki" {
  name              = "deepwiki"
  server_url        = "https://mcp.deepwiki.com/mcp"
  server_identifier = "deepwiki"
  icon              = "🌐"
  icon_type         = "emoji"

  headers = {
    Authorization = "Bearer ${var.mcp_token}"
  }
}

variable "mcp_token" {
  type      = string
  sensitive = true
  default   = ""
}
