# Install a plugin from the Dify marketplace. The identifier is versioned:
# "<plugin>:<version>@<sha256>". Installation is asynchronous; the resource
# polls the install task until it completes.
resource "dify_plugin_install" "openai" {
  plugin_unique_identifier = "langgenius/openai:0.3.8@592c8252795b5f75807de2d609a03196ed02596b409f7642b4a07548c7ff57ef"
}
