# Manage a Dify application from its exported DSL YAML.
# Export the DSL from the Dify console (or GET /openapi/v1/apps/<id>/dsl),
# then point dsl_yaml at the file.
resource "dify_app" "my_workflow" {
  dsl_yaml    = file("${path.module}/my-workflow.yaml")
  name        = "My Workflow"
  description = "A workflow managed by Terraform"

  # Publish the imported draft (workflow/advanced-chat apps only)
  publish = true
}
