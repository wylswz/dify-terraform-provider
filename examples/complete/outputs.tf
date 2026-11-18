output "research_app_id" {
  value = dify_app.research.id
}

output "translate_app_id" {
  value = dify_app.translate.id
}

output "weather_brief_app_id" {
  value = dify_app.weather_brief.id
}

output "orchestrator_app_id" {
  value = dify_app.orchestrator.id
}

output "web_research_tool_id" {
  value = dify_workflow_tool.web_research.id
}

output "translate_text_tool_id" {
  value = dify_workflow_tool.translate_text.id
}

output "weather_brief_tool_id" {
  value = dify_workflow_tool.weather_brief.id
}

output "deepwiki_mcp_provider_id" {
  value = dify_mcp_provider.deepwiki.id
}

output "orchestrator_api_key" {
  value     = dify_app_api_key.orchestrator.token
  sensitive = true
}
