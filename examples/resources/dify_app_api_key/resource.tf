resource "dify_app" "my_workflow" {
  dsl_yaml = file("${path.module}/my-workflow.yaml")
}

# Create a Service API key for the app. The token is only available
# at create time and is stored (sensitively) in Terraform state.
resource "dify_app_api_key" "key" {
  app_id = dify_app.my_workflow.id
}
