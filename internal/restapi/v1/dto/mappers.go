package dto

import (
	"fmt"

	"windshift/internal/models"
	"windshift/internal/services"
)

// MapUserToSummary converts a models.User to a UserSummary DTO
func MapUserToSummary(user *models.User) *UserSummary {
	if user == nil {
		return nil
	}
	fullName := user.FullName
	if fullName == "" {
		fullName = user.FirstName + " " + user.LastName
	}
	return &UserSummary{
		ID:        user.ID,
		Email:     user.Email,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		FullName:  fullName,
		AvatarURL: user.AvatarURL,
	}
}

// MapUserSummaryFromItem creates a UserSummary from item's joined user fields
func MapUserSummaryFromItem(id *int, name, email string) *UserSummary {
	if id == nil || *id == 0 {
		return nil
	}
	return &UserSummary{
		ID:       *id,
		FullName: name,
		Email:    email,
	}
}

// MapItemToResponse converts a models.Item to an ItemResponse DTO
func MapItemToResponse(item *models.Item, baseURL string) *ItemResponse {
	if item == nil {
		return nil
	}

	resp := &ItemResponse{
		ID:                  item.ID,
		WorkspaceID:         item.WorkspaceID,
		WorkspaceKey:        item.WorkspaceKey,
		Key:                 fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber),
		WorkspaceItemNumber: item.WorkspaceItemNumber,
		Title:               item.Title,
		Description:         item.Description,
		IsTask:              item.IsTask,
		DueDate:             item.DueDate,
		CustomFields:        item.CustomFieldValues,
		ParentID:            item.ParentID,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
		CompletedAt:         item.CompletedAt,
	}

	// Map status
	if item.StatusID != nil {
		resp.Status = &StatusSummary{
			ID:            *item.StatusID,
			Name:          item.StatusName,
			CategoryColor: "", // Would need to be populated from expanded data
		}
	}

	// Map priority
	if item.PriorityID != nil {
		resp.Priority = &PrioritySummary{
			ID:    *item.PriorityID,
			Name:  item.PriorityName,
			Icon:  item.PriorityIcon,
			Color: item.PriorityColor,
		}
	}

	// Map item type
	if item.ItemTypeID != nil {
		resp.ItemType = &ItemTypeSummary{
			ID:   *item.ItemTypeID,
			Name: item.ItemTypeName,
		}
	}

	// Map assignee
	if item.AssigneeID != nil {
		resp.Assignee = &UserSummary{
			ID:        *item.AssigneeID,
			FullName:  item.AssigneeName,
			Email:     item.AssigneeEmail,
			AvatarURL: item.AssigneeAvatar,
		}
	}

	// Map creator
	if item.CreatorID != nil {
		resp.Creator = &UserSummary{
			ID:       *item.CreatorID,
			FullName: item.CreatorName,
			Email:    item.CreatorEmail,
		}
	}

	// Map workspace
	if item.WorkspaceName != "" {
		resp.Workspace = &WorkspaceSummary{
			ID:   item.WorkspaceID,
			Name: item.WorkspaceName,
			Key:  item.WorkspaceKey,
		}
	}

	// Map milestone
	if item.MilestoneID != nil {
		resp.Milestone = &MilestoneSummary{
			ID:   *item.MilestoneID,
			Name: item.MilestoneName,
		}
	}

	// Map iteration
	if item.IterationID != nil {
		resp.Iteration = &IterationSummary{
			ID:   *item.IterationID,
			Name: item.IterationName,
		}
	}

	// Map project
	if item.ProjectID != nil {
		resp.Project = &ProjectSummary{
			ID:   *item.ProjectID,
			Name: item.ProjectName,
		}
	}

	// Add HATEOAS links
	if baseURL != "" {
		resp.Links = &ItemLinks{
			Self:        fmt.Sprintf("%s/rest/api/v1/items/%d", baseURL, item.ID),
			Workspace:   fmt.Sprintf("%s/rest/api/v1/workspaces/%d", baseURL, item.WorkspaceID),
			Comments:    fmt.Sprintf("%s/rest/api/v1/items/%d/comments", baseURL, item.ID),
			History:     fmt.Sprintf("%s/rest/api/v1/items/%d/history", baseURL, item.ID),
			Attachments: fmt.Sprintf("%s/rest/api/v1/items/%d/attachments", baseURL, item.ID),
			Children:    fmt.Sprintf("%s/rest/api/v1/items/%d/children", baseURL, item.ID),
			Transitions: fmt.Sprintf("%s/rest/api/v1/items/%d/transitions", baseURL, item.ID),
		}
		if item.ParentID != nil {
			resp.Links.Parent = fmt.Sprintf("%s/rest/api/v1/items/%d", baseURL, *item.ParentID)
		}
	}

	return resp
}

// MapItemsToResponse converts a slice of models.Item to ItemResponse DTOs
func MapItemsToResponse(items []models.Item, baseURL string) []ItemResponse {
	result := make([]ItemResponse, len(items))
	for i := range items {
		result[i] = *MapItemToResponse(&items[i], baseURL)
	}
	return result
}

// MapCommentToResponse converts a models.Comment to a CommentResponse DTO
func MapCommentToResponse(comment *models.Comment) *CommentResponse {
	if comment == nil {
		return nil
	}

	resp := &CommentResponse{
		ID:        comment.ID,
		ItemID:    comment.ItemID,
		Content:   comment.Content,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}

	if comment.AuthorID != nil {
		resp.Author = &UserSummary{
			ID:       *comment.AuthorID,
			FullName: comment.AuthorName,
			Email:    comment.AuthorEmail,
		}
	} else if comment.AuthorName != "" {
		// Portal customer comment - set author info from joined fields
		resp.Author = &UserSummary{
			FullName: comment.AuthorName,
			Email:    comment.AuthorEmail,
		}
	}

	return resp
}

// MapCommentsToResponse converts a slice of models.Comment to CommentResponse DTOs
func MapCommentsToResponse(comments []models.Comment) []CommentResponse {
	result := make([]CommentResponse, len(comments))
	for i := range comments {
		result[i] = *MapCommentToResponse(&comments[i])
	}
	return result
}

// MapHistoryToResponse converts a models.ItemHistory to a HistoryResponse DTO
func MapHistoryToResponse(history *models.ItemHistory) *HistoryResponse {
	if history == nil {
		return nil
	}

	resp := &HistoryResponse{
		ID:               history.ID,
		ItemID:           history.ItemID,
		FieldName:        history.FieldName,
		OldValue:         history.OldValue,
		NewValue:         history.NewValue,
		ResolvedOldValue: history.ResolvedOldValue,
		ResolvedNewValue: history.ResolvedNewValue,
		ChangedAt:        history.ChangedAt,
	}

	if history.UserID != 0 {
		resp.User = &UserSummary{
			ID:       history.UserID,
			FullName: history.UserName,
			Email:    history.UserEmail,
		}
	}

	return resp
}

// MapHistoryToResponses converts a slice of models.ItemHistory to HistoryResponse DTOs
func MapHistoryToResponses(histories []models.ItemHistory) []HistoryResponse {
	result := make([]HistoryResponse, len(histories))
	for i := range histories {
		result[i] = *MapHistoryToResponse(&histories[i])
	}
	return result
}

// MapAttachmentToResponse converts a models.Attachment to an AttachmentResponse DTO
func MapAttachmentToResponse(attachment *models.Attachment, baseURL string) *AttachmentResponse {
	if attachment == nil {
		return nil
	}

	resp := &AttachmentResponse{
		ID:               attachment.ID,
		ItemID:           attachment.ItemID,
		Filename:         attachment.Filename,
		OriginalFilename: attachment.OriginalFilename,
		MimeType:         attachment.MimeType,
		FileSize:         attachment.FileSize,
		HasThumbnail:     attachment.HasThumbnail,
		CreatedAt:        attachment.CreatedAt,
	}

	if attachment.UploadedBy != nil {
		resp.Uploader = &UserSummary{
			ID:       *attachment.UploadedBy,
			FullName: attachment.UploaderName,
			Email:    attachment.UploaderEmail,
		}
	}

	if baseURL != "" {
		resp.DownloadURL = fmt.Sprintf("%s/rest/api/v1/attachments/%d/download", baseURL, attachment.ID)
		if attachment.HasThumbnail {
			resp.ThumbnailURL = fmt.Sprintf("%s/rest/api/v1/attachments/%d/thumbnail", baseURL, attachment.ID)
		}
	}

	return resp
}

// MapAttachmentsToResponse converts a slice of models.Attachment to AttachmentResponse DTOs
func MapAttachmentsToResponse(attachments []models.Attachment, baseURL string) []AttachmentResponse {
	result := make([]AttachmentResponse, len(attachments))
	for i := range attachments {
		result[i] = *MapAttachmentToResponse(&attachments[i], baseURL)
	}
	return result
}

// MapWorkflowTransitionToResponse converts a models.WorkflowTransition to TransitionResponse DTO
func MapWorkflowTransitionToResponse(transition *models.WorkflowTransition) *TransitionResponse {
	if transition == nil {
		return nil
	}

	resp := &TransitionResponse{
		ID:           transition.ID,
		FromStatusID: transition.FromStatusID,
		ToStatusID:   transition.ToStatusID,
	}

	if transition.FromStatusID != nil {
		resp.FromStatus = &StatusSummary{
			ID:   *transition.FromStatusID,
			Name: transition.FromStatusName,
		}
	}

	resp.ToStatus = &StatusSummary{
		ID:   transition.ToStatusID,
		Name: transition.ToStatusName,
	}

	return resp
}

// MapTransitionsToResponse converts a slice of models.WorkflowTransition to TransitionResponse DTOs
func MapTransitionsToResponse(transitions []models.WorkflowTransition) []TransitionResponse {
	result := make([]TransitionResponse, len(transitions))
	for i := range transitions {
		result[i] = *MapWorkflowTransitionToResponse(&transitions[i])
	}
	return result
}

// MapServiceTransitionsToResponse converts WorkflowTransitionResult from services to TransitionResponse DTOs
func MapServiceTransitionsToResponse(transitions []services.WorkflowTransitionResult) []TransitionResponse {
	result := make([]TransitionResponse, len(transitions))
	for i, t := range transitions {
		resp := TransitionResponse{
			ID:           t.ID,
			FromStatusID: t.FromStatusID,
			ToStatusID:   t.ToStatusID,
		}

		if t.FromStatusID != nil {
			resp.FromStatus = &StatusSummary{
				ID:   *t.FromStatusID,
				Name: t.FromStatusName,
			}
		}

		resp.ToStatus = &StatusSummary{
			ID:   t.ToStatusID,
			Name: t.ToStatusName,
		}

		result[i] = resp
	}
	return result
}
