# Acme Analytics — Integrations

Acme Analytics connects to external systems through the integrations panel.

## Supported Sources

- S3 and GCS buckets (batch and change-data-capture modes)
- Postgres, MySQL, and SQL Server (logical replication)
- Salesforce and HubSpot (managed connectors, OAuth)
- Webhooks for real-time event ingestion

## API Access

Every workspace exposes a REST API for programmatic access. API keys are
scoped per workspace and can be rotated without downtime. Rate limits are
1,000 requests per minute per key.
