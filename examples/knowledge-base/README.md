# Knowledge Base + Retrieval Example

A complete RAG recall scenario managed entirely by Terraform: a knowledge
base (`dify_dataset`), documents imported from local Markdown files
(`dify_dataset_document` + `dify_dataset_document_indexing_wait`), and a
published workflow app (`dify_app`) whose `knowledge-retrieval` node searches
the dataset.

## Layout

- `main.tf` — dataset, per-file documents via `fileset`, indexing waits, and
  the retrieval app
- `variables.tf` / `outputs.tf`
- `docs/*.md` — the corpus; edit and re-apply to update the knowledge base
- `dsls/retrieval.yml.tftpl` — workflow DSL template; `${dataset_id}` is
  injected with `templatefile()` because the dataset id only exists at apply
  time

## Usage

```bash
export DIFY_SERVER=https://dify.example.com
export DIFY_TOKEN=...        # Personal Access Token
export DIFY_WORKSPACE_ID=...  # optional; empty = the token owner's default workspace

terraform init
terraform apply \
  -var dify_token=$DIFY_TOKEN \
  -var dify_server=$DIFY_SERVER \
  -var dify_workspace_id=$DIFY_WORKSPACE_ID
```

`indexing_technique` defaults to `economy` (keyword indexing, no embedding
model required). Set `-var indexing_technique=high_quality` for vector
search once the workspace has a default embedding model configured.

## Running the workflow

The published app can be executed through the OpenAPI run endpoint:

```bash
curl -N -X POST "$DIFY_SERVER/openapi/v1/apps/$(terraform output -raw retrieval_app_id)/workflow:run" \
  -H "Authorization: Bearer $DIFY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"inputs": {"query": "how is billing calculated"}}'
```

The response is an SSE stream; the `workflow_finished` event's `data.outputs`
contains `result` — the retrieved segments with their source document names
and scores.

### Running against a locally built provider

`wylswz/dify` is not published, so point Terraform at a local `make
install` build with a `dev_overrides` CLI config — see
`examples/complete/README.md` for the snippet.
