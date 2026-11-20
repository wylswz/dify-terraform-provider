resource "dify_mcp_provider" "deepwiki" {
  name       = "deepwiki"
  server_url = "https://mcp.deepwiki.com/mcp"
}

# Filter by provider ID...
data "dify_tools" "deepwiki" {
  provider_name = dify_mcp_provider.deepwiki.id
}

# ...or search by keywords matched against name, label, and provider.
data "dify_tools" "search" {
  query = "search"
}
