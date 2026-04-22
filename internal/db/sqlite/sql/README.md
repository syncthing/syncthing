These SQL scripts are embedded in the binary.

Scripts in `schema/` are run at every startup, in alphanumerical order.

Scripts in `migrations/` are run when a migration is needed; they must begin
with a number that equals the schema version that results from that
migration. Migrations are not run on initial database creation, so the
scripts in `schema/` should create the latest version.
