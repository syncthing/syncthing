package sqlite

import (
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

const currentSchemaVersion = 1

type schemaVersion struct {
	SchemaVersion    int
	AppliedAt        int64
	SyncthingVersion string
}

func (s *schemaVersion) AppliedTime() time.Time {
	return time.Unix(0, s.AppliedAt)
}

func (s *DB) setAppliedSchemaVersion(ver int) error {
	_, err := s.stmt(`
		INSERT OR IGNORE INTO schemamigrations (schema_version, applied_at, syncthing_version)
		VALUES (?, ?, ?)
	`).Exec(ver, time.Now().UnixNano(), build.LongVersion)
	return wrap(err)
}

func (s *DB) getAppliedSchemaVersion() (schemaVersion, error) {
	var v schemaVersion
	err := s.stmt(`
		SELECT schema_version as schemaversion, applied_at as appliedat, syncthing_version as syncthingversion FROM schemamigrations
		ORDER BY schema_version DESC
		LIMIT 1
	`).Get(&v)
	return v, wrap(err)
}
