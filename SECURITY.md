# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities privately — do not open a public issue.

Preferred channel: use GitHub's **private vulnerability reporting** on this
repository (Security tab → "Report a vulnerability"), if it is enabled. If it
is not available, contact the repository owner
[@wylswz](https://github.com/wylswz) privately via GitHub.

Please include:

- a description of the vulnerability and its impact;
- steps to reproduce or a proof of concept;
- the provider version and, if relevant, the Dify version affected.

Reports are handled on a best-effort basis. You can expect an acknowledgment
within a few days.

## Handling secrets

The provider token (a Dify Personal Access Token) should be passed via the
`DIFY_TOKEN` environment variable or a sensitive Terraform variable — it is
not persisted in state. However, resource-level secrets that Dify returns
(such as `dify_app_api_key` tokens, credential secrets, and MCP headers) are
marked `Sensitive` but still stored in Terraform state and plan files. Treat
state and plan output as secret material: restrict access to the state
backend and never paste raw state or plan output into issues or pull
requests.
