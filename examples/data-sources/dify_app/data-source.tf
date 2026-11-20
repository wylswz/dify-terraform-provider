# Look up an app by ID or by exact name (exactly one is required).
data "dify_app" "by_id" {
  id = "00000000-0000-0000-0000-000000000000"
}

data "dify_app" "by_name" {
  name = "My Workflow"
}
