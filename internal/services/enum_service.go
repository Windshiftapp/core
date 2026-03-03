package services

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
)

// EnumEntity is the interface that all enum models must implement
type EnumEntity interface {
	GetID() int
	GetName() string
}

// ValidationFunc validates an entity before create/update
// Returns error message if validation fails, empty string if valid
type ValidationFunc func(entity interface{}, isUpdate bool) string

// UniqueCheckFunc checks if an entity with the same unique key exists
// For create: excludeID is 0
// For update: excludeID is the ID being updated
// Returns true if duplicate exists
type UniqueCheckFunc func(db database.Database, entity interface{}, excludeID int) (bool, error)

// FKValidationFunc validates foreign key references exist
// Returns error message if FK is invalid, empty string if valid
type FKValidationFunc func(db database.Database, entity interface{}) string

// DeleteCheckFunc checks if entity can be deleted (no dependencies)
// Returns error message describing dependency, empty string if deletable
type DeleteCheckFunc func(db database.Database, id int) string

// BeforeDeleteFunc runs before delete (e.g., check system protection)
// Returns (shouldProceed, httpStatusCode, errorMessage)
type BeforeDeleteFunc func(db database.Database, id int) (bool, int, string)

// BeforeUpdateFunc runs before update (e.g., check system protection)
// Returns (shouldProceed, httpStatusCode, errorMessage)
type BeforeUpdateFunc func(db database.Database, id int, entity interface{}) (bool, int, string)

// AfterCreateFunc runs after successful create (e.g., junction tables, audit)
type AfterCreateFunc func(db database.Database, id int, entity interface{}, r *http.Request) error

// AfterUpdateFunc runs after successful update (e.g., junction tables, audit)
type AfterUpdateFunc func(db database.Database, id int, entity interface{}, r *http.Request) error

// AfterDeleteFunc runs after successful delete (e.g., audit logging)
type AfterDeleteFunc func(db database.Database, id int, name string, r *http.Request) error

// ScanRowFunc scans a database row into an entity
type ScanRowFunc func(rows *sql.Rows) (EnumEntity, error)

// ScanSingleRowFunc scans a single row (QueryRow result) into an entity
type ScanSingleRowFunc func(row *sql.Row) (EnumEntity, error)

// InsertArgsFunc returns the insert query columns and arguments
// Returns (columns string, placeholders string, args []interface{})
type InsertArgsFunc func(entity interface{}, now time.Time) (string, string, []interface{})

// UpdateArgsFunc returns the update SET clause and arguments
// Returns (setClause string, args []interface{}) - args should NOT include id (added by service)
type UpdateArgsFunc func(entity interface{}, now time.Time) (string, []interface{})

// DefaultValueFunc applies default values to entity before insert
type DefaultValueFunc func(entity interface{})

// EnumConfig defines the configuration for a generic enum CRUD service
type EnumConfig struct {
	// Table and entity info
	TableName  string
	EntityName string // For error messages (e.g., "Status category")

	// SQL Queries
	SelectColumns string // Columns to select (e.g., "id, name, color, description")
	SelectQuery   string // Full SELECT query for GetAll (including JOINs if needed)
	GetByIDQuery  string // Query to get single entity by ID

	// Row scanning
	ScanRow       ScanRowFunc
	ScanSingleRow ScanSingleRowFunc

	// Insert/Update
	InsertArgs InsertArgsFunc
	UpdateArgs UpdateArgsFunc

	// Validation
	Validate    ValidationFunc   // Optional
	CheckUnique UniqueCheckFunc  // Optional
	ValidateFKs FKValidationFunc // Optional

	// Delete handling
	CheckDependencies DeleteCheckFunc  // Optional
	BeforeDelete      BeforeDeleteFunc // Optional

	// Update handling
	BeforeUpdate BeforeUpdateFunc // Optional

	// Post-operation hooks
	AfterCreate AfterCreateFunc // Optional
	AfterUpdate AfterUpdateFunc // Optional
	AfterDelete AfterDeleteFunc // Optional

	// Defaults
	ApplyDefaults DefaultValueFunc // Optional

	// Ordering
	DefaultOrderBy string // e.g., "name ASC" or "level ASC"
}

// EnumService provides generic CRUD operations for enum-like entities
type EnumService struct {
	db     database.Database
	config EnumConfig
}

// NewEnumService creates a new enum service with the given configuration
func NewEnumService(db database.Database, config EnumConfig) *EnumService {
	return &EnumService{db: db, config: config}
}

// ServiceError represents a service-layer error with HTTP status
type ServiceError struct {
	StatusCode int
	Message    string
}

func (e *ServiceError) Error() string {
	return e.Message
}

// NewServiceError creates a new service error
func NewServiceError(statusCode int, message string) *ServiceError {
	return &ServiceError{StatusCode: statusCode, Message: message}
}

// GetAll retrieves all entities
func (s *EnumService) GetAll() ([]EnumEntity, error) {
	query := s.config.SelectQuery
	if query == "" {
		//nolint:gosec // G201: TableName, SelectColumns, DefaultOrderBy are from hardcoded EnumConfig, not user input
		query = fmt.Sprintf("SELECT %s FROM %s ORDER BY %s",
			s.config.SelectColumns, s.config.TableName, s.config.DefaultOrderBy)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []EnumEntity
	for rows.Next() {
		entity, err := s.config.ScanRow(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}

	// Always return empty slice, not nil
	if entities == nil {
		entities = []EnumEntity{}
	}

	return entities, nil
}

// GetByID retrieves a single entity by ID
func (s *EnumService) GetByID(id int) (EnumEntity, error) {
	query := s.config.GetByIDQuery
	if query == "" {
		//nolint:gosec // G201: TableName, SelectColumns are from hardcoded EnumConfig, not user input
		query = fmt.Sprintf("SELECT %s FROM %s WHERE id = ?",
			s.config.SelectColumns, s.config.TableName)
	}

	row := s.db.QueryRow(query, id)
	entity, err := s.config.ScanSingleRow(row)

	if err == sql.ErrNoRows {
		return nil, NewServiceError(http.StatusNotFound,
			fmt.Sprintf("%s not found", s.config.EntityName))
	}
	if err != nil {
		return nil, err
	}

	return entity, nil
}

// Create creates a new entity
func (s *EnumService) Create(entity interface{}, r *http.Request) (EnumEntity, error) {
	// Apply defaults
	if s.config.ApplyDefaults != nil {
		s.config.ApplyDefaults(entity)
	}

	// Validate
	if s.config.Validate != nil {
		if errMsg := s.config.Validate(entity, false); errMsg != "" {
			return nil, NewServiceError(http.StatusBadRequest, errMsg)
		}
	}

	// Validate FKs
	if s.config.ValidateFKs != nil {
		if errMsg := s.config.ValidateFKs(s.db, entity); errMsg != "" {
			return nil, NewServiceError(http.StatusBadRequest, errMsg)
		}
	}

	// Check uniqueness
	if s.config.CheckUnique != nil {
		exists, err := s.config.CheckUnique(s.db, entity, 0)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, NewServiceError(http.StatusConflict,
				fmt.Sprintf("%s with this name already exists", s.config.EntityName))
		}
	}

	// Insert
	now := time.Now()
	columns, placeholders, args := s.config.InsertArgs(entity, now)

	//nolint:gosec // G201: TableName, columns, placeholders are from hardcoded EnumConfig, not user input
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		s.config.TableName, columns, placeholders)

	query += " RETURNING id"
	var id int64
	err := s.db.QueryRow(query, args...).Scan(&id)
	if err != nil {
		// Check for unique constraint violation (database-level)
		if isUniqueConstraintError(err) {
			return nil, NewServiceError(http.StatusConflict,
				fmt.Sprintf("%s already exists", s.config.EntityName))
		}
		return nil, err
	}

	// Run after-create hook
	if s.config.AfterCreate != nil {
		if err := s.config.AfterCreate(s.db, int(id), entity, r); err != nil {
			return nil, err
		}
	}

	// Re-query to get full entity with timestamps
	return s.GetByID(int(id))
}

// Update updates an existing entity
func (s *EnumService) Update(id int, entity interface{}, r *http.Request) (EnumEntity, error) {
	// Check entity exists first
	_, err := s.GetByID(id)
	if err != nil {
		return nil, err // Returns 404 if not found
	}

	// Before update hook (e.g., system protection)
	if s.config.BeforeUpdate != nil {
		proceed, statusCode, errMsg := s.config.BeforeUpdate(s.db, id, entity)
		if !proceed {
			return nil, NewServiceError(statusCode, errMsg)
		}
	}

	// Validate
	if s.config.Validate != nil {
		if errMsg := s.config.Validate(entity, true); errMsg != "" {
			return nil, NewServiceError(http.StatusBadRequest, errMsg)
		}
	}

	// Validate FKs
	if s.config.ValidateFKs != nil {
		if errMsg := s.config.ValidateFKs(s.db, entity); errMsg != "" {
			return nil, NewServiceError(http.StatusBadRequest, errMsg)
		}
	}

	// Check uniqueness (excluding current record)
	if s.config.CheckUnique != nil {
		var exists bool
		exists, err = s.config.CheckUnique(s.db, entity, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, NewServiceError(http.StatusConflict,
				fmt.Sprintf("%s with this name already exists", s.config.EntityName))
		}
	}

	// Update
	now := time.Now()
	setClause, args := s.config.UpdateArgs(entity, now)
	args = append(args, id) // Add ID at the end for WHERE clause

	//nolint:gosec // G201: TableName, setClause are from hardcoded EnumConfig, not user input
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		s.config.TableName, setClause)

	_, err = s.db.Exec(query, args...)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, NewServiceError(http.StatusConflict,
				fmt.Sprintf("%s already exists", s.config.EntityName))
		}
		return nil, err
	}

	// Run after-update hook
	if s.config.AfterUpdate != nil {
		if err := s.config.AfterUpdate(s.db, id, entity, r); err != nil {
			return nil, err
		}
	}

	// Re-query to get updated entity
	return s.GetByID(id)
}

// Delete deletes an entity by ID
func (s *EnumService) Delete(id int, r *http.Request) error {
	// Check entity exists and get name for audit
	existing, err := s.GetByID(id)
	if err != nil {
		return err // Returns 404 if not found
	}

	// Before delete hook (e.g., system protection)
	if s.config.BeforeDelete != nil {
		proceed, statusCode, errMsg := s.config.BeforeDelete(s.db, id)
		if !proceed {
			return NewServiceError(statusCode, errMsg)
		}
	}

	// Check dependencies
	if s.config.CheckDependencies != nil {
		if errMsg := s.config.CheckDependencies(s.db, id); errMsg != "" {
			return NewServiceError(http.StatusConflict, errMsg)
		}
	}

	// Delete
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", s.config.TableName) //nolint:gosec // G201: TableName is from hardcoded EnumConfig, not user input
	_, err = s.db.Exec(query, id)
	if err != nil {
		return err
	}

	// Run after-delete hook
	if s.config.AfterDelete != nil {
		if err := s.config.AfterDelete(s.db, id, existing.GetName(), r); err != nil {
			return err
		}
	}

	return nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique_violation")
}
