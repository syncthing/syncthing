-- Schema migrations hold the list of historical migrations applied
CREATE TABLE IF NOT EXISTS schemamigrations (
    schema_version INTEGER NOT NULL,
    applied_at INTEGER NOT NULL, -- unix nanos
    syncthing_version TEXT NOT NULL COLLATE BINARY,
    PRIMARY KEY(schema_version)
) STRICT
;
