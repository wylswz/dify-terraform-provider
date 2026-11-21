# Acme Analytics — Permissions & Access Control

Access to Acme Analytics is controlled at three levels: workspace, database,
and table.

## Roles

- **Owner** — full control, including billing and member management.
- **Admin** — manage databases, users, and integrations; cannot change billing.
- **Member** — query data and create dashboards; cannot modify schemas.
- **Viewer** — read-only access to shared dashboards.

## Row-Level Security

Row-level security policies filter rows per user based on attributes such as
region or team. Policies are defined in YAML and applied at query time, so
there is no performance penalty for large tables.
