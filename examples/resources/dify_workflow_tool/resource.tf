# The workflow app must be published before it can become a tool.
resource "dify_app" "research" {
  dsl_yaml = file("${path.module}/research.yaml")
  publish  = true
}

# Publish the workflow app as a workspace tool callable by agents.
resource "dify_workflow_tool" "web_research" {
  workflow_app_id = dify_app.research.id
  name            = "web_research"
  label           = "Web Research"
  description     = "Research a question on the web"
  icon = {
    content    = "🔎"
    background = "#FFEAD5"
  }

  parameters = [
    { name = "query", description = "The question to research", form = "llm" },
  ]
}
