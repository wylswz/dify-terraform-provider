# Acme Analytics — Product Documentation

Acme Analytics provides a managed data warehouse and BI platform. This
document covers the core concepts every user should know.

## Data Warehouse

The Acme warehouse stores curated datasets in columnar tables. Data lands
through batch loads (nightly) or streaming ingestion (sub-minute latency).
Tables are organized into databases, and databases into workspaces.

## Querying

Users query data with AcmeQL, a SQL dialect with window functions and
approximate percentile aggregates. Queries run against cached hot data by
default; cold storage queries take longer and are billed at a lower rate.

## Billing

Workspaces are billed monthly by bytes scanned. The first 100 GB scanned
per month is included in every plan. Query history and cost reports are
available under Settings → Billing.
