package persistence

import "database/sql"

// Database is the subset of the application database used by WebAuthn
// persistence. The concrete credential and session schemas are selected by
// the fixed constructors in this package, never by a caller-provided value.
type Database interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecWrite(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowWrite(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}
