package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// CollectionRecord is the read projection needed by REST v1 collection embeds.
type CollectionRecord struct {
	ID          int
	Slug        string
	Name        string
	Description string
	QLQuery     string
	WorkspaceID *int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	IsPublic    bool
	IsPrivate   bool
	CreatedBy   *int
}

// CollectionRepository owns collection metadata lookups.
type CollectionRepository struct {
	db database.Database
}

// NewCollectionRepository creates a collection repository.
func NewCollectionRepository(db database.Database) *CollectionRepository {
	return &CollectionRepository{db: db}
}

const collectionModelSelect = `SELECT c.id, c.name, COALESCE(c.description, ''), COALESCE(c.ql_query, ''),
       c.filter_state, c.is_public, c.is_private, c.public_milestone_swimlanes, c.workspace_id, c.category_id, c.created_by,
       c.public_slug, c.created_at, c.updated_at,
       COALESCE(u.first_name || ' ' || u.last_name, ''), COALESCE(u.email, ''),
       COALESCE(cc.name, ''), COALESCE(cc.color, '')
FROM collections c
LEFT JOIN users u ON c.created_by = u.id
LEFT JOIN collection_categories cc ON c.category_id = cc.id`

type collectionModelScanner interface {
	Scan(dest ...any) error
}

func scanCollectionModel(scanner collectionModelScanner) (models.Collection, error) {
	var collection models.Collection
	var workspaceID, categoryID, createdBy sql.NullInt64
	var publicSlug, filterState sql.NullString
	err := scanner.Scan(
		&collection.ID, &collection.Name, &collection.Description, &collection.QLQuery,
		&filterState, &collection.IsPublic, &collection.IsPrivate, &collection.PublicMilestoneSwimlanes,
		&workspaceID, &categoryID, &createdBy,
		&publicSlug, &collection.CreatedAt, &collection.UpdatedAt,
		&collection.CreatorName, &collection.CreatorEmail,
		&collection.CategoryName, &collection.CategoryColor,
	)
	if err != nil {
		return models.Collection{}, err
	}
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		collection.WorkspaceID = &id
	}
	if categoryID.Valid {
		id := int(categoryID.Int64)
		collection.CategoryID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		collection.CreatedBy = &id
	}
	if publicSlug.Valid {
		collection.PublicSlug = &publicSlug.String
	}
	if filterState.Valid {
		collection.FilterState = &filterState.String
	}
	return collection, nil
}

// CollectionListFilter narrows the cookie-auth collection list.
type CollectionListFilter struct {
	UserID      int
	WorkspaceID *int
	CategoryID  *int
}

// ListVisibleModels returns candidate collections for the caller: shared
// collections plus private collections they own. Shared workspace-linked rows
// still need the caller's workspace access, which this layer has no context
// for; the service narrows them.
func (r *CollectionRepository) ListVisibleModels(filter CollectionListFilter) ([]models.Collection, error) {
	query := collectionModelSelect + "\nWHERE (c.is_private = false OR c.created_by = ?)"
	args := []any{filter.UserID}
	if filter.WorkspaceID != nil {
		query += " AND c.workspace_id = ?"
		args = append(args, *filter.WorkspaceID)
	}
	if filter.CategoryID != nil {
		query += " AND c.category_id = ?"
		args = append(args, *filter.CategoryID)
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list visible collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	collections := make([]models.Collection, 0)
	for rows.Next() {
		collection, err := scanCollectionModel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan visible collection: %w", err)
		}
		collections = append(collections, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read visible collections: %w", err)
	}
	return collections, nil
}

// GetVisibleModel returns a shared collection or a private one owned by the
// caller. Workspace permission is enforced by the service.
func (r *CollectionRepository) GetVisibleModel(id, userID int) (*models.Collection, error) {
	collection, err := scanCollectionModel(r.db.QueryRow(
		collectionModelSelect+"\nWHERE c.id = ? AND (c.is_private = false OR c.created_by = ?)", id, userID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get visible collection %d: %w", id, err)
	}
	return &collection, nil
}

// GetModel returns the full editable collection model without a visibility
// filter. Authorization remains the handler's responsibility.
func (r *CollectionRepository) GetModel(id int) (*models.Collection, error) {
	collection, err := scanCollectionModel(r.db.QueryRow(collectionModelSelect+"\nWHERE c.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection model %d: %w", id, err)
	}
	return &collection, nil
}

// FindByWorkspaceAndName returns an exact collection match, or ErrNotFound.
func (r *CollectionRepository) FindByWorkspaceAndName(workspaceID int, name string) (*models.Collection, error) {
	collection, err := scanCollectionModel(r.db.QueryRow(
		collectionModelSelect+"\nWHERE c.workspace_id = ? AND c.name = ?", workspaceID, name,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find collection %q in workspace %d: %w", name, workspaceID, err)
	}
	return &collection, nil
}

// CreateForImport inserts an optionally unowned collection.
func (r *CollectionRepository) CreateForImport(collection *models.Collection, userID *int) error {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO collections
			(name, description, ql_query, filter_state, is_public, is_private, public_milestone_swimlanes, workspace_id, category_id, created_by, public_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, collection.Name, collection.Description, collection.QLQuery, collection.FilterState,
		collection.IsPublic, collection.IsPrivate, collection.PublicMilestoneSwimlanes, collection.WorkspaceID, collection.CategoryID, userID, collection.PublicSlug).Scan(&id)
	if err != nil {
		return fmt.Errorf("create imported collection: %w", err)
	}
	collection.ID = int(id)
	collection.CreatedBy = userID
	return nil
}

// GetOwnerID returns the creator ID. A nil creator represents an unowned
// legacy collection.
func (r *CollectionRepository) GetOwnerID(id int) (*int, error) {
	var ownerID sql.NullInt64
	err := r.db.QueryRow("SELECT created_by FROM collections WHERE id = ?", id).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection %d owner: %w", id, err)
	}
	if !ownerID.Valid {
		return nil, nil
	}
	idValue := int(ownerID.Int64)
	return &idValue, nil
}

// CategoryExists reports whether a collection category exists.
func (r *CollectionRepository) CategoryExists(id int) (bool, error) {
	var exists bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM collection_categories WHERE id = ?)", id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check collection category %d: %w", id, err)
	}
	return exists, nil
}

// Create inserts a collection and populates its generated ID and owner.
func (r *CollectionRepository) Create(collection *models.Collection, userID int) error {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO collections (name, description, ql_query, filter_state, is_public, is_private, public_milestone_swimlanes, workspace_id, category_id, created_by, public_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, collection.Name, collection.Description, collection.QLQuery, collection.FilterState,
		collection.IsPublic, collection.IsPrivate, collection.PublicMilestoneSwimlanes, collection.WorkspaceID, collection.CategoryID, userID, collection.PublicSlug).Scan(&id)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	collection.ID = int(id)
	collection.CreatedBy = &userID
	return nil
}

// Update replaces all editable collection fields.
func (r *CollectionRepository) Update(id int, collection *models.Collection) error {
	err := database.WithTx(r.db, func(tx database.Tx) error {
		// Serialize scope changes with queue creation. Updating only updated_at
		// locks the row without firing the collection change trigger, which
		// watches query, filter, and workspace columns.
		var lockedID int
		err := tx.QueryRow(`
			UPDATE collections SET updated_at = updated_at WHERE id = ?
			RETURNING id
		`, id).Scan(&lockedID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			UPDATE collections
			SET name = ?, description = ?, ql_query = ?, filter_state = ?, is_public = ?, is_private = ?, public_milestone_swimlanes = ?, workspace_id = ?, category_id = ?, public_slug = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, collection.Name, collection.Description, collection.QLQuery, collection.FilterState,
			collection.IsPublic, collection.IsPrivate, collection.PublicMilestoneSwimlanes, collection.WorkspaceID, collection.CategoryID, collection.PublicSlug, id); err != nil {
			return err
		}
		// Keep the informational workspace reference on collection-scoped queues
		// in sync when the collection moves between the workspace and global
		// scopes. The queue scope itself is the collection id.
		if collection.WorkspaceID != nil {
			if _, err := tx.Exec(`
				UPDATE queues SET workspace_id = ?, updated_at = CURRENT_TIMESTAMP
				WHERE collection_id = ? AND COALESCE(workspace_id, 0) != ?
			`, *collection.WorkspaceID, id, *collection.WorkspaceID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(`
				UPDATE queues SET workspace_id = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE collection_id = ? AND workspace_id IS NOT NULL
			`, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update collection %d: %w", id, err)
	}
	return nil
}

// UpdatePublicSharing updates only public-board visibility fields. When
// publishing, the update applies only while the stored scope still matches
// expectedQuery and expectedWorkspaceID, so a concurrent query edit cannot
// publish a scope that was never authorized. The bool reports whether the row
// still matched.
func (r *CollectionRepository) UpdatePublicSharing(id int, isPublic bool, publicSlug *string, publicMilestoneSwimlanes bool, expectedQuery string, expectedWorkspaceID *int) (bool, error) {
	query := "UPDATE collections SET is_public = ?, public_slug = ?, public_milestone_swimlanes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	args := []any{isPublic, publicSlug, publicMilestoneSwimlanes, id}
	if isPublic {
		query += " AND COALESCE(ql_query, '') = ? AND COALESCE(workspace_id, 0) = COALESCE(?, 0)"
		args = append(args, expectedQuery, expectedWorkspaceID)
	}
	result, err := r.db.ExecWrite(query, args...)
	if err != nil {
		return false, fmt.Errorf("update collection %d public sharing: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read collection %d public sharing rows: %w", id, err)
	}
	return affected > 0, nil
}

// Delete removes a collection.
func (r *CollectionRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM collections WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete collection %d: %w", id, err)
	}
	return nil
}

// GetByID fetches a collection by numeric id.
func (r *CollectionRepository) GetByID(id int) (*CollectionRecord, error) {
	var rec CollectionRecord
	var workspaceID sql.NullInt64
	var createdBy sql.NullInt64
	var slug sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, COALESCE(description, ''), COALESCE(ql_query, ''), workspace_id, is_public, is_private, created_by,
		       public_slug, created_at, updated_at
		FROM collections
		WHERE id = ?
	`, id).Scan(
		&rec.ID, &rec.Name, &rec.Description, &rec.QLQuery,
		&workspaceID, &rec.IsPublic, &rec.IsPrivate, &createdBy,
		&slug, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection by id: %w", err)
	}
	hydrateCollectionNulls(&rec, workspaceID, createdBy)
	if slug.Valid {
		rec.Slug = slug.String
	}
	return &rec, nil
}

// GetBySlug fetches a collection by public slug.
func (r *CollectionRepository) GetBySlug(slug string) (*CollectionRecord, error) {
	var rec CollectionRecord
	var workspaceID sql.NullInt64
	var createdBy sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, name, COALESCE(description, ''), workspace_id, is_public, is_private, created_by, created_at, updated_at
		FROM collections
		WHERE public_slug = ? AND public_slug IS NOT NULL
	`, slug).Scan(
		&rec.ID, &rec.Name, &rec.Description,
		&workspaceID, &rec.IsPublic, &rec.IsPrivate, &createdBy,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get collection by slug: %w", err)
	}
	rec.Slug = slug
	hydrateCollectionNulls(&rec, workspaceID, createdBy)
	return &rec, nil
}

// SearchByName returns a bounded candidate set for case-insensitive collection
// name search. Callers still apply per-row visibility filtering.
func (r *CollectionRepository) SearchByName(q string, limit int) ([]CollectionRecord, error) {
	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := r.db.Query(`
		SELECT id, name, COALESCE(description, ''), workspace_id, is_public, is_private, created_by,
		       COALESCE(public_slug, ''), created_at, updated_at
		FROM collections
		WHERE LOWER(name) LIKE ?
		ORDER BY name ASC
		LIMIT ?
	`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := []CollectionRecord{}
	for rows.Next() {
		var rec CollectionRecord
		var workspaceID sql.NullInt64
		var createdBy sql.NullInt64
		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.Description,
			&workspaceID, &rec.IsPublic, &rec.IsPrivate, &createdBy,
			&rec.Slug, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		hydrateCollectionNulls(&rec, workspaceID, createdBy)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collections: %w", err)
	}
	return records, nil
}

func hydrateCollectionNulls(rec *CollectionRecord, workspaceID, createdBy sql.NullInt64) {
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		rec.WorkspaceID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		rec.CreatedBy = &id
	}
}
