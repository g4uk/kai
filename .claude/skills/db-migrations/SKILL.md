---
name: db-migrations
description: >
  Database migration rules. Use when a task touches the DB schema:
  new table, ALTER TABLE, indexes, migrations (goose/ActiveRecord/etc).
---
# DB Migrations

<!-- EDIT_ME: tool and naming for your project -->

1. Every migration has a down. No exceptions.
2. Backward-compatible by default:
   add column nullable → deploy → backfill → NOT NULL as a separate migration.
3. DROP TABLE / DROP COLUMN — only after explicit human approval,
   as a separate PR, one release after usage stops.
4. Indexes on large tables — CONCURRENTLY (separate migration, outside a transaction).
5. [multi-tenant] New tenant table = company_id NOT NULL + index from day one.
6. Migrations are NEVER run by an agent against prod/staging — local only.
