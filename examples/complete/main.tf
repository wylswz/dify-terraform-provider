terraform {
  required_providers {
    dify = {
      source  = "langgenius/dify"
      version = "0.1.0"
    }
  }
}

provider "dify" {
  host         = var.dify_server
  token        = var.dify_token
  workspace_id = var.dify_workspace_id
}

# ---------------------------------------------------------------------------
# Plugins
# ---------------------------------------------------------------------------
resource "dify_plugin_install" "openai" {
  plugin_unique_identifier = "langgenius/openai:0.3.8@592c8252795b5f75807de2d609a03196ed02596b409f7642b4a07548c7ff57ef"
}

resource "dify_plugin_install" "tavily" {
  plugin_unique_identifier = "langgenius/tavily:0.1.7@5fce9cf01fecda9ad92e5125397d2bb5497429baed276c7f14f033e7debd0abe"
}

locals {
  # The backup credential falls back to the primary key when unset.
  openai_api_key_backup = coalesce(var.openai_api_key_backup, var.openai_api_key)
}

# ---------------------------------------------------------------------------
# Models: credentials, defaults, load balancing
# ---------------------------------------------------------------------------
resource "dify_model_provider_credential" "openai" {
  provider_name = "langgenius/openai/openai"
  name          = "production-key"
  make_active   = true
  credentials = {
    openai_api_key = var.openai_api_key
  }

  depends_on = [dify_plugin_install.openai]
}

resource "dify_model_provider_credential" "openai_backup" {
  provider_name = "langgenius/openai/openai"
  name          = "backup-key"
  credentials = {
    openai_api_key = local.openai_api_key_backup
  }

  depends_on = [dify_plugin_install.openai]
}

resource "dify_default_model" "llm" {
  model_type    = "llm"
  provider_name = dify_model_provider_credential.openai.provider_name
  model         = "gpt-4o-mini"

  depends_on = [dify_model_provider_credential.openai]
}

resource "dify_default_model" "embedding" {
  model_type    = "text-embedding"
  provider_name = dify_model_provider_credential.openai.provider_name
  model         = "text-embedding-3-small"

  depends_on = [dify_model_provider_credential.openai]
}

resource "dify_model_settings" "gpt4o_mini" {
  provider_name          = dify_model_provider_credential.openai.provider_name
  model                  = "gpt-4o-mini"
  model_type             = "llm"
  config_from            = "predefined-model"
  enabled                = true
  load_balancing_enabled = true

  # The null credential_id entry reuses the model's active credential;
  # the second entry spreads load onto the backup credential.
  load_balancing_entries = [
    {
      enabled = true
    },
    {
      credential_id = dify_model_provider_credential.openai_backup.credential_id
      enabled       = true
    },
  ]

  depends_on = [
    dify_model_provider_credential.openai,
    dify_model_provider_credential.openai_backup,
  ]
}

# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------
resource "dify_tool_provider_credential" "tavily" {
  provider_name = "langgenius/tavily/tavily"
  name          = "production-key"
  credentials = {
    tavily_api_key = var.tavily_api_key
  }

  depends_on = [dify_plugin_install.tavily]
}

resource "dify_api_tool_provider" "open_meteo" {
  name = "open_meteo"
  icon = {
    content    = "⛅"
    background = "#E0F2FE"
  }
  schema_type = "openapi"
  schema      = file("${path.module}/dsls/open_meteo_schema.json")
  credentials = { auth_type = "none" }
}

resource "dify_mcp_provider" "deepwiki" {
  name              = "deepwiki"
  server_url        = var.mcp_server_url
  server_identifier = "deepwiki"
  icon              = "🌐"
  icon_type         = "emoji"
}

# ---------------------------------------------------------------------------
# Workflow apps, published so they can be tools
# ---------------------------------------------------------------------------
resource "dify_app" "research" {
  dsl_yaml    = file("${path.module}/dsls/research.yml")
  name        = "Research"
  description = "Search the web with Tavily and summarize the findings"
  publish     = true

  depends_on = [
    dify_model_provider_credential.openai,
    dify_tool_provider_credential.tavily,
  ]
}

resource "dify_app" "translate" {
  dsl_yaml    = file("${path.module}/dsls/translate.yml")
  name        = "Translate"
  description = "Translate text into a target language"
  publish     = true

  depends_on = [dify_model_provider_credential.openai]
}

resource "dify_app" "weather_brief" {
  dsl_yaml = templatefile("${path.module}/dsls/weather_brief.yml.tftpl", {
    open_meteo_tool = local.open_meteo_node
  })
  name        = "Weather Brief"
  description = "Fetch current weather for coordinates and summarize it"
  publish     = true

  depends_on = [
    dify_model_provider_credential.openai,
    dify_api_tool_provider.open_meteo,
  ]
}

resource "dify_workflow_tool" "web_research" {
  workflow_app_id = dify_app.research.id
  name            = "web_research"
  label           = "Web Research"
  description     = "Research a question on the web and summarize the findings"
  icon = {
    content    = "🔎"
    background = "#FFEAD5"
  }
  parameters = [
    {
      name        = "query"
      description = "The question to research"
      form        = "llm"
    },
  ]
}

resource "dify_workflow_tool" "translate_text" {
  workflow_app_id = dify_app.translate.id
  name            = "translate_text"
  label           = "Translate Text"
  description     = "Translate text into a target language"
  icon = {
    content    = "🌐"
    background = "#E4FBCC"
  }
  parameters = [
    {
      name        = "text"
      description = "The text to translate"
      form        = "llm"
    },
    {
      name        = "target_language"
      description = "The language to translate into"
      form        = "llm"
    },
  ]
}

resource "dify_workflow_tool" "weather_brief" {
  workflow_app_id = dify_app.weather_brief.id
  name            = "weather_brief"
  label           = "Weather Brief"
  description     = "Get a friendly weather brief for coordinates"
  icon = {
    content    = "⛅"
    background = "#E0F2FE"
  }
  parameters = [
    {
      name        = "latitude"
      description = "Latitude of the location"
      form        = "llm"
    },
    {
      name        = "longitude"
      description = "Longitude of the location"
      form        = "llm"
    },
  ]
}

# Tool rows with their server-provided node data, read after the providers
# exist so DSL templates can use the exact provider_id/provider_name/
# provider_type the server expects.
data "dify_tools" "deepwiki" {
  provider_name = dify_mcp_provider.deepwiki.id
}

# API tool providers expose a server DB id rather than their name, so filter
# by words and pick the api-typed tool below.
data "dify_tools" "open_meteo" {
  query = "open_meteo"

  # No attribute references the provider, so without this the read would run
  # at plan time, before the API tool provider exists.
  depends_on = [dify_api_tool_provider.open_meteo]
}

data "dify_tools" "web_research" {
  provider_name = dify_workflow_tool.web_research.id
}

data "dify_tools" "translate_text" {
  provider_name = dify_workflow_tool.translate_text.id
}

data "dify_tools" "weather_brief" {
  provider_name = dify_workflow_tool.weather_brief.id
}

locals {
  # one() fails loudly on zero or several matches instead of rendering a broken DSL.
  # query is a substring match, so match the exact provider name inside node_data
  # to ignore look-alike providers (e.g. lc_open_meteo_v2).
  open_meteo_node = jsondecode(one(
    [for t in data.dify_tools.open_meteo.tools : t.node_data
    if t.provider_type == "api" && jsondecode(t.node_data).provider_name == dify_api_tool_provider.open_meteo.name],
  ))
  mcp_tool_nodes = [for t in data.dify_tools.deepwiki.tools : jsondecode(t.node_data)]
  workflow_tool_nodes = [
    for t in concat(
      data.dify_tools.web_research.tools,
      data.dify_tools.translate_text.tools,
      data.dify_tools.weather_brief.tools,
    ) : jsondecode(t.node_data)
  ]
}

# ---------------------------------------------------------------------------
# Orchestrator agent app
# ---------------------------------------------------------------------------
resource "dify_app" "orchestrator" {
  dsl_yaml = templatefile("${path.module}/dsls/orchestrator.yml.tftpl", {
    workflow_tools = local.workflow_tool_nodes
    api_tool       = local.open_meteo_node
    mcp_tools      = local.mcp_tool_nodes
  })
  name                = "Research Assistant"
  description         = "An agent that researches, translates, and reports the weather"
  service_api_enabled = true
}

resource "dify_app_api_key" "orchestrator" {
  app_id = dify_app.orchestrator.id
}
