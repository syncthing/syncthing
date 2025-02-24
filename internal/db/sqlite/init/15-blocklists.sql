-- Block lists
--
-- The blocklists are extracted from FileInfos and stored separately when
-- they're over a certain size. This reduces the database size by reusing
-- the same blocklist entry for all devices announcing the same file.
--
-- We keep a reference counter, incremented in the code and decremented by
-- triggers. When it reaches zero the blocklist is deleted.
CREATE TABLE IF NOT EXISTS blocklists (
    blocklist_hash BLOB NOT NULL PRIMARY KEY,
    refcount INTEGER NOT NULL,
    blprotobuf BLOB NOT NULL
) STRICT
;

-- Decrement refcount when a referencing file entry is deleted
CREATE TRIGGER IF NOT EXISTS blocklist_refcount_decrease AFTER DELETE ON files
BEGIN
    UPDATE blocklists SET refcount = refcount - 1 WHERE blocklist_hash = OLD.blocklist_hash;
END
;

-- Delete the blocklist when the refcount reaches zero
CREATE TRIGGER IF NOT EXISTS blocklist_refcount_cleanup AFTER UPDATE ON blocklists
WHEN NEW.refcount = 0
BEGIN
    DELETE FROM blocklists WHERE blocklist_hash = NEW.blocklist_hash AND refcount = 0;
END
;
