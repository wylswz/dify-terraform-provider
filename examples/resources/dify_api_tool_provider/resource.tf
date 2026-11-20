# Register a custom tool provider from an OpenAPI schema document.
resource "dify_api_tool_provider" "open_meteo" {
  name = "open_meteo"
  icon = {
    content    = "⛅"
    background = "#E0F2FE"
  }
  schema_type = "openapi"
  schema      = file("${path.module}/openapi.json")
  credentials = { auth_type = "none" }
}
