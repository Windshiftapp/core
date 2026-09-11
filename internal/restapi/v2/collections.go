package v2

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/services"
)

func registerCollectionRoutes(builder *routeBuilder, collections collectionApplication) {
	builder.Page("/collection-categories", AuthAuthenticated, []string{"collections:read"}, listCollectionCategories(collections))
	builder.JSON(http.MethodPost, "/collection-categories", http.StatusCreated, false, AuthAuthenticated, []string{"collections:write"}, createCollectionCategory(collections))
	builder.Read("/collection-categories/{category_id}", AuthAuthenticated, []string{"collections:read"}, getCollectionCategory(collections))
	builder.JSON(http.MethodPatch, "/collection-categories/{category_id}", http.StatusOK, true, AuthAuthenticated, []string{"collections:write"}, patchCollectionCategory(collections))
	builder.Command(http.MethodDelete, "/collection-categories/{category_id}", AuthAuthenticated, []string{"collections:delete"}, deleteCollectionCategory(collections))
	builder.Page("/collections", AuthAuthenticated, []string{"collections:read"}, listCollections(collections))
	builder.JSON(http.MethodPost, "/collections", http.StatusCreated, false, AuthAuthenticated, []string{"collections:write"}, createCollection(collections))
	builder.Read("/collections/{collection_id}", AuthAuthenticated, []string{"collections:read"}, getCollection(collections))
	builder.JSON(http.MethodPatch, "/collections/{collection_id}", http.StatusOK, true, AuthAuthenticated, []string{"collections:write"}, updateCollection(collections))
	builder.JSON(http.MethodPatch, "/collections/{collection_id}/sharing", http.StatusOK, true, AuthAuthenticated, []string{"collections:write"}, updateCollectionSharing(collections))
	builder.Command(http.MethodDelete, "/collections/{collection_id}", AuthAuthenticated, []string{"collections:delete"}, deleteCollection(collections))
	registerBoardConfigurationRoutes(builder, collections, "/collections/{collection_id}", "collection_id")
	registerBoardConfigurationRoutes(builder, collections, "/workspaces/{workspace_id}", "workspace_id")
}

func registerBoardConfigurationRoutes(builder *routeBuilder, collections collectionApplication, base, scopeParam string) {
	builder.Read(base+"/board-configuration", AuthAuthenticated, []string{"collections:read"}, getBoardConfiguration(collections, scopeParam))
	builder.Read(base+"/board-configuration/bootstrap", AuthAuthenticated, []string{"collections:read"}, getBoardConfigurationBootstrap(collections, scopeParam))
	builder.JSON(http.MethodPut, base+"/board-configuration", http.StatusOK, false, AuthAuthenticated, []string{"collections:write"}, putBoardConfiguration(collections, scopeParam))
	builder.Command(http.MethodDelete, base+"/board-configuration", AuthAuthenticated, []string{"collections:delete"}, deleteBoardConfiguration(collections, scopeParam))
}

type collectionPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	QLQuery     Optional[string] `json:"ql_query"`
	FilterState Optional[string] `json:"filter_state"`
	WorkspaceID Optional[int]    `json:"workspace_id"`
	CategoryID  Optional[int]    `json:"category_id"`
	IsPublic    Optional[bool]   `json:"is_public"`
	PublicSlug  Optional[string] `json:"public_slug"`
}

type collectionSharingRequest struct {
	IsPublic   Optional[bool]   `json:"is_public"`
	PublicSlug Optional[string] `json:"public_slug"`
}

type collectionCategoryPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Color       Optional[string] `json:"color"`
	Description Optional[string] `json:"description"`
}

func listCollectionCategories(collections collectionApplication) pageOperation[models.CollectionCategory] {
	return func(r *http.Request) ([]models.CollectionCategory, Pagination, int, error) {
		page, err := ParsePage(r)
		if err != nil {
			return nil, page, 0, err
		}
		items, err := collections.ListCategories()
		if err != nil {
			return nil, page, 0, collectionError(err)
		}
		result, total := paginate(items, page)
		return result, page, total, nil
	}
}

func getCollectionCategory(collections collectionApplication) readOperation[models.CollectionCategory] {
	return func(r *http.Request) (models.CollectionCategory, error) {
		id, err := pathID(r, "category_id")
		if err != nil {
			return models.CollectionCategory{}, err
		}
		item, err := collections.GetCategory(id)
		if item == nil {
			return models.CollectionCategory{}, collectionError(err)
		}
		return *item, collectionError(err)
	}
}

func createCollectionCategory(collections collectionApplication) jsonOperation[models.CollectionCategory, models.CollectionCategory] {
	return func(r *http.Request, input models.CollectionCategory) (models.CollectionCategory, error) {
		user, err := principal(r)
		if err != nil {
			return models.CollectionCategory{}, err
		}
		item, err := collections.CreateCategory(auditActor(r, user), input)
		if item == nil {
			return models.CollectionCategory{}, collectionError(err)
		}
		return *item, collectionError(err)
	}
}

func patchCollectionCategory(collections collectionApplication) jsonOperation[collectionCategoryPatchRequest, models.CollectionCategory] {
	return func(r *http.Request, input collectionCategoryPatchRequest) (models.CollectionCategory, error) {
		user, err := principal(r)
		if err != nil {
			return models.CollectionCategory{}, err
		}
		id, err := pathID(r, "category_id")
		if err != nil {
			return models.CollectionCategory{}, err
		}
		item, err := collections.PatchCategory(auditActor(r, user), id, services.CollectionCategoryPatch{
			Name: optionalValue(input.Name), Color: optionalValue(input.Color), Description: optionalValue(input.Description),
		})
		if item == nil {
			return models.CollectionCategory{}, collectionError(err)
		}
		return *item, collectionError(err)
	}
}

func deleteCollectionCategory(collections collectionApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "category_id")
		if err != nil {
			return err
		}
		return collectionError(collections.DeleteCategory(auditActor(r, user), id))
	}
}

func listCollections(collections collectionApplication) pageOperation[models.Collection] {
	return func(r *http.Request) ([]models.Collection, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		workspaceID, err := optionalPositiveQueryID(r, "workspace_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		categoryID, err := optionalPositiveQueryID(r, "category_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		result, total, err := collections.List(services.CollectionListParams{
			Query:  r.URL.Query().Get("q"),
			UserID: user.ID, WorkspaceID: workspaceID, CategoryID: categoryID, Limit: page.PageSize, Offset: page.Offset,
		})
		return result, page, total, collectionError(err)
	}
}

func getCollection(collections collectionApplication) readOperation[models.Collection] {
	return func(r *http.Request) (models.Collection, error) {
		user, err := principal(r)
		if err != nil {
			return models.Collection{}, err
		}
		key := r.PathValue("collection_id")
		var collection *models.Collection
		if _, parseErr := strconv.Atoi(key); parseErr == nil {
			id, idErr := pathID(r, "collection_id")
			if idErr != nil {
				return models.Collection{}, idErr
			}
			collection, err = collections.Get(user.ID, id)
		} else {
			collection, err = collections.GetBySlug(user.ID, key)
		}
		if collection == nil {
			return models.Collection{}, collectionError(err)
		}
		return *collection, collectionError(err)
	}
}

func createCollection(collections collectionApplication) jsonOperation[models.Collection, models.Collection] {
	return func(r *http.Request, input models.Collection) (models.Collection, error) {
		user, err := principal(r)
		if err != nil {
			return models.Collection{}, err
		}
		created, err := collections.Create(auditActor(r, user), input)
		if created == nil {
			return models.Collection{}, collectionError(err)
		}
		return *created, collectionError(err)
	}
}

func updateCollection(collections collectionApplication) jsonOperation[collectionPatchRequest, models.Collection] {
	return func(r *http.Request, input collectionPatchRequest) (models.Collection, error) {
		user, err := principal(r)
		if err != nil {
			return models.Collection{}, err
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return models.Collection{}, err
		}
		updated, err := collections.Update(auditActor(r, user), id, services.CollectionUpdate{
			NameSet: input.Name.Set, Name: input.Name.Value,
			DescriptionSet: input.Description.Set, Description: input.Description.Value,
			QLQuerySet: input.QLQuery.Set, QLQuery: input.QLQuery.Value,
			FilterStateSet: input.FilterState.Set, FilterState: optionalString(input.FilterState),
			WorkspaceIDSet: input.WorkspaceID.Set, WorkspaceID: optionalInt(input.WorkspaceID),
			CategoryIDSet: input.CategoryID.Set, CategoryID: optionalInt(input.CategoryID),
			IsPublicSet: input.IsPublic.Set, IsPublic: input.IsPublic.Value,
			PublicSlugSet: input.PublicSlug.Set, PublicSlug: optionalString(input.PublicSlug),
		})
		if updated == nil {
			return models.Collection{}, collectionError(err)
		}
		return *updated, collectionError(err)
	}
}

func updateCollectionSharing(collections collectionApplication) jsonOperation[collectionSharingRequest, models.Collection] {
	return func(r *http.Request, input collectionSharingRequest) (models.Collection, error) {
		user, err := principal(r)
		if err != nil {
			return models.Collection{}, err
		}
		if !input.IsPublic.Set {
			return models.Collection{}, newError(http.StatusBadRequest, "invalid_request", "is_public is required")
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return models.Collection{}, err
		}
		updated, err := collections.UpdateSharing(auditActor(r, user), id, services.CollectionSharingUpdate{
			IsPublic: input.IsPublic.Value, PublicSlug: optionalString(input.PublicSlug),
		})
		if updated == nil {
			return models.Collection{}, collectionError(err)
		}
		return *updated, collectionError(err)
	}
}

func deleteCollection(collections collectionApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "collection_id")
		if err != nil {
			return err
		}
		return collectionError(collections.Delete(auditActor(r, user), id))
	}
}

func getBoardConfiguration(collections collectionApplication, scopeParam string) readOperation[models.BoardConfiguration] {
	return func(r *http.Request) (models.BoardConfiguration, error) {
		user, scope, err := boardConfigurationContext(r, scopeParam)
		if err != nil {
			return models.BoardConfiguration{}, err
		}
		config, err := collections.GetBoardConfiguration(user.ID, scope)
		if config == nil {
			return models.BoardConfiguration{}, collectionError(err)
		}
		return *config, collectionError(err)
	}
}

func getBoardConfigurationBootstrap(collections collectionApplication, scopeParam string) readOperation[services.BoardConfigurationBootstrap] {
	return func(r *http.Request) (services.BoardConfigurationBootstrap, error) {
		user, scope, err := boardConfigurationContext(r, scopeParam)
		if err != nil {
			return services.BoardConfigurationBootstrap{}, err
		}
		var fallbackWorkspaceID *int
		if scope.CollectionID != nil {
			fallbackWorkspaceID, err = optionalPositiveQueryID(r, "workspace_id")
			if err != nil {
				return services.BoardConfigurationBootstrap{}, err
			}
		}
		bootstrap, err := collections.GetBoardConfigurationBootstrap(r.Context(), user.ID, scope, fallbackWorkspaceID)
		if bootstrap == nil {
			return services.BoardConfigurationBootstrap{}, collectionError(err)
		}
		return *bootstrap, collectionError(err)
	}
}

func putBoardConfiguration(collections collectionApplication, scopeParam string) jsonOperation[models.BoardConfigurationRequest, models.BoardConfiguration] {
	return func(r *http.Request, input models.BoardConfigurationRequest) (models.BoardConfiguration, error) {
		user, scope, err := boardConfigurationContext(r, scopeParam)
		if err != nil {
			return models.BoardConfiguration{}, err
		}
		config, err := collections.PutBoardConfiguration(auditActor(r, user), scope, input)
		if config == nil {
			return models.BoardConfiguration{}, collectionError(err)
		}
		return *config, collectionError(err)
	}
}

func deleteBoardConfiguration(collections collectionApplication, scopeParam string) commandOperation {
	return func(r *http.Request) error {
		user, scope, err := boardConfigurationContext(r, scopeParam)
		if err != nil {
			return err
		}
		return collectionError(collections.DeleteBoardConfiguration(auditActor(r, user), scope))
	}
}

func boardConfigurationContext(r *http.Request, scopeParam string) (*models.User, services.BoardConfigurationScope, error) {
	user, err := principal(r)
	if err != nil {
		return nil, services.BoardConfigurationScope{}, err
	}
	id, err := pathID(r, scopeParam)
	if err != nil {
		return nil, services.BoardConfigurationScope{}, err
	}
	scope := services.BoardConfigurationScope{}
	if scopeParam == "collection_id" {
		scope.CollectionID = &id
	} else {
		scope.WorkspaceID = &id
	}
	return user, scope, nil
}

func optionalPositiveQueryID(r *http.Request, name string) (*int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return nil, newError(http.StatusBadRequest, "invalid_request", name+" must be a positive integer")
	}
	return &id, nil
}

func optionalInt(value Optional[int]) *int {
	if !value.Set || value.Null {
		return nil
	}
	result := value.Value
	return &result
}

func optionalString(value Optional[string]) *string {
	if !value.Set || value.Null {
		return nil
	}
	result := value.Value
	return &result
}

func collectionError(err error) error {
	if err == nil {
		return nil
	}
	var validation *services.CollectionValidationError
	var serviceError *services.ServiceError
	switch {
	case errors.Is(err, services.ErrCollectionNotFound), errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusNotFound:
		return newError(http.StatusNotFound, "not_found", "Collection was not found")
	case errors.Is(err, services.ErrCollectionForbidden):
		return newError(http.StatusForbidden, "permission_denied", "Collection access was denied")
	case errors.Is(err, services.ErrCollectionConflict), errors.Is(err, services.ErrBoardConflict), errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusConflict:
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusBadRequest:
		return newError(http.StatusBadRequest, "invalid_request", serviceError.Message)
	case errors.As(err, &validation):
		return newError(http.StatusBadRequest, "invalid_request", validation.Message)
	default:
		return internalError(err)
	}
}
