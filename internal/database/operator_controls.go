package database

import "windshift/internal/operatorcontrols"

type controlledDatabase struct {
	Database
	controls *operatorcontrols.Controls
}

func (db *controlledDatabase) OperatorControls() *operatorcontrols.Controls { return db.controls }

func (db *controlledDatabase) InstanceID() uint64 {
	if source, ok := db.Database.(interface{ InstanceID() uint64 }); ok {
		return source.InstanceID()
	}
	return 0
}

// WithOperatorControls carries one instance's immutable deployment policy to
// shared services, including services constructed by tools and background jobs.
func WithOperatorControls(db Database, controls *operatorcontrols.Controls) Database {
	return &controlledDatabase{Database: db, controls: controls}
}
