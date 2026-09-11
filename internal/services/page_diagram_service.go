package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/fileserve"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const (
	PageDiagramPlacementStart = "start"
	PageDiagramPlacementEnd   = "end"
)

var (
	ErrPageDiagramNotFound          = errors.New("page diagram not found")
	ErrPageDiagramNameRequired      = errors.New("page diagram name is required")
	ErrPageDiagramPlacementInvalid  = errors.New("page diagram placement must be start or end")
	ErrPageDiagramReferenceConflict = errors.New("page diagram attachment must be referenced exactly once")
)

type PageDiagram struct {
	PageID         int             `json:"page_id"`
	AttachmentID   int             `json:"attachment_id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Payload        json.RawMessage `json:"payload,omitempty" swaggertype:"object"`
	ContentHash    string          `json:"content_hash,omitempty"`
	RevisionNumber int             `json:"revision_number,omitempty"`
	CreatedAt      time.Time       `json:"created_at,omitempty"`
}

type CreatePageDiagramInput struct {
	PageID              int             `json:"-"`
	Name                string          `json:"name"`
	Mermaid             string          `json:"mermaid,omitempty"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty" swaggertype:"object"`
	Placement           string          `json:"placement"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty"`
}

type UpdatePageDiagramInput struct {
	PageID              int             `json:"-"`
	AttachmentID        int             `json:"-"`
	Name                string          `json:"name,omitempty"`
	Mermaid             string          `json:"mermaid,omitempty"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty" swaggertype:"object"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty"`
}

type pageDiagramBlock struct {
	start        int
	end          int
	attachmentID int
	name         string
}

var pageDiagramFencePattern = regexp.MustCompile("(?m)^```excalidraw[ \\t]*\\r?\\n([^\\r\\n]*)\\r?\\n```[ \\t]*$")

// PageDiagramService owns the attachment-backed Page diagram lifecycle. Page
// revisions keep immutable attachment IDs; only failed mutations are cleaned
// up, never superseded successful versions.
type PageDiagramService struct {
	db             database.Database
	attachmentPath string
	pages          *PageApplicationService
	pageAuth       *PagePermissionService
	uploads        *PageAttachmentUploadService
	attachments    *repository.AttachmentRepository
}

func NewPageDiagramService(
	db database.Database,
	attachmentPath string,
	pages *PageApplicationService,
	pageAuth *PagePermissionService,
	permissionService *PermissionService,
) *PageDiagramService {
	return &PageDiagramService{
		db:             db,
		attachmentPath: attachmentPath,
		pages:          pages,
		pageAuth:       pageAuth,
		uploads:        NewPageAttachmentUploadService(db, attachmentPath, permissionService, pageAuth),
		attachments:    repository.NewAttachmentRepository(db),
	}
}

func (s *PageDiagramService) Create(actor AuditActor, in CreatePageDiagramInput) (*PageDiagram, error) {
	name := sanitize.ShortIdentifier.Sanitize(in.Name)
	if name == "" {
		return nil, ErrPageDiagramNameRequired
	}
	if in.Placement != PageDiagramPlacementStart && in.Placement != PageDiagramPlacementEnd {
		return nil, ErrPageDiagramPlacementInvalid
	}
	payload, kind, err := BuildDiagramPayload(in.Mermaid, in.Excalidraw)
	if err != nil {
		return nil, err
	}
	page, err := s.requirePage(actor.UserID, in.PageID, PageOpEdit)
	if err != nil {
		return nil, err
	}
	uploaded, err := s.uploads.UploadPageAttachment(PageAttachmentUploadInput{
		PageID:           page.ID,
		UploaderID:       actor.UserID,
		OriginalFilename: "diagram.json",
		FileData:         []byte(payload),
		FileSize:         int64(len(payload)),
	})
	if err != nil {
		return nil, err
	}
	attachmentID := uploaded.Attachment.ID
	content := insertPageDiagramFence(page.Content, renderPageDiagramFence(attachmentID, name), in.Placement)
	updated, err := s.pages.Update(actor, page.WorkspaceID, PageApplicationUpdateInput{
		ID:                  page.ID,
		Content:             &content,
		ExpectedContentHash: in.ExpectedContentHash,
	})
	if err != nil {
		return nil, s.compensateFailedMutation(page.ID, attachmentID, err)
	}
	return &PageDiagram{
		PageID:         page.ID,
		AttachmentID:   attachmentID,
		Name:           name,
		Kind:           kind,
		ContentHash:    updated.ContentHash,
		RevisionNumber: s.latestRevisionNumber(page.ID),
		CreatedAt:      uploaded.Attachment.CreatedAt,
	}, nil
}

func (s *PageDiagramService) List(actor AuditActor, pageID int) ([]PageDiagram, error) {
	page, err := s.requirePage(actor.UserID, pageID, PageOpView)
	if err != nil {
		return nil, err
	}
	blocks := parsePageDiagramBlocks(page.Content)
	out := make([]PageDiagram, 0, len(blocks))
	for _, block := range blocks {
		rec, payload, kind, readErr := s.readPageDiagramPayload(page.ID, block.attachmentID)
		if errors.Is(readErr, repository.ErrNotFound) || errors.Is(readErr, ErrPageDiagramNotFound) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		out = append(out, PageDiagram{
			PageID:       page.ID,
			AttachmentID: block.attachmentID,
			Name:         block.name,
			Kind:         kind,
			Payload:      payload,
			ContentHash:  page.ContentHash,
			CreatedAt:    rec.CreatedAt,
		})
	}
	return out, nil
}

func (s *PageDiagramService) Get(actor AuditActor, pageID, attachmentID int) (*PageDiagram, error) {
	page, err := s.requirePage(actor.UserID, pageID, PageOpView)
	if err != nil {
		return nil, err
	}
	blocks := matchingPageDiagramBlocks(page.Content, attachmentID)
	if len(blocks) == 0 {
		return nil, ErrPageDiagramNotFound
	}
	if len(blocks) != 1 {
		return nil, ErrPageDiagramReferenceConflict
	}
	rec, payload, kind, err := s.readPageDiagramPayload(page.ID, attachmentID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrPageDiagramNotFound
	}
	if err != nil {
		return nil, err
	}
	return &PageDiagram{
		PageID:       page.ID,
		AttachmentID: attachmentID,
		Name:         blocks[0].name,
		Kind:         kind,
		Payload:      payload,
		ContentHash:  page.ContentHash,
		CreatedAt:    rec.CreatedAt,
	}, nil
}

func (s *PageDiagramService) Update(actor AuditActor, in UpdatePageDiagramInput) (*PageDiagram, error) {
	payload, kind, err := BuildDiagramPayload(in.Mermaid, in.Excalidraw)
	if err != nil {
		return nil, err
	}
	page, err := s.requirePage(actor.UserID, in.PageID, PageOpEdit)
	if err != nil {
		return nil, err
	}
	blocks := matchingPageDiagramBlocks(page.Content, in.AttachmentID)
	if len(blocks) == 0 {
		return nil, ErrPageDiagramNotFound
	}
	if len(blocks) != 1 {
		return nil, ErrPageDiagramReferenceConflict
	}
	if _, err := s.attachments.GetPageAttachmentRecord(page.ID, in.AttachmentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPageDiagramNotFound
		}
		return nil, err
	}
	uploaded, err := s.uploads.UploadPageAttachment(PageAttachmentUploadInput{
		PageID:           page.ID,
		UploaderID:       actor.UserID,
		OriginalFilename: "diagram.json",
		FileData:         []byte(payload),
		FileSize:         int64(len(payload)),
	})
	if err != nil {
		return nil, err
	}
	attachmentID := uploaded.Attachment.ID
	block := blocks[0]
	name := block.name
	if requestedName := sanitize.ShortIdentifier.Sanitize(in.Name); requestedName != "" {
		name = requestedName
	}
	replacement := renderPageDiagramFence(attachmentID, name)
	content := page.Content[:block.start] + replacement + page.Content[block.end:]
	updated, err := s.pages.Update(actor, page.WorkspaceID, PageApplicationUpdateInput{
		ID:                  page.ID,
		Content:             &content,
		ExpectedContentHash: in.ExpectedContentHash,
	})
	if err != nil {
		return nil, s.compensateFailedMutation(page.ID, attachmentID, err)
	}
	return &PageDiagram{
		PageID:         page.ID,
		AttachmentID:   attachmentID,
		Name:           name,
		Kind:           kind,
		ContentHash:    updated.ContentHash,
		RevisionNumber: s.latestRevisionNumber(page.ID),
		CreatedAt:      uploaded.Attachment.CreatedAt,
	}, nil
}

func (s *PageDiagramService) requirePage(userID, pageID int, op string) (*models.Page, error) {
	if s.pages == nil || s.pageAuth == nil {
		return nil, ErrPageDiagramNotFound
	}
	page, err := s.pages.PageService().GetByID(pageID)
	if err != nil {
		return nil, ErrPageDiagramNotFound
	}
	allowed, err := s.pageAuth.Can(userID, page.WorkspaceID, page.ID, op)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPageDiagramNotFound
	}
	return page, nil
}

func (s *PageDiagramService) readPageDiagramPayload(pageID, attachmentID int) (record *repository.PageAttachmentRecord, payload json.RawMessage, kind string, err error) {
	record, err = s.attachments.GetPageAttachmentRecord(pageID, attachmentID)
	if err != nil {
		return nil, nil, "", err
	}
	file, err := fileserve.OpenUnderRoot(s.attachmentPath, record.FilePath)
	if err != nil {
		return nil, nil, "", ErrPageDiagramNotFound
	}
	defer func() { _ = file.Close() }()
	payloadBytes, err := io.ReadAll(io.LimitReader(file, MaxDiagramPayloadBytes+1))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read page diagram attachment: %w", err)
	}
	payload = json.RawMessage(payloadBytes)
	kind, err = ValidateStoredDiagramPayload(payload)
	if err != nil {
		return nil, nil, "", err
	}
	return record, payload, kind, nil
}

func (s *PageDiagramService) compensateFailedMutation(pageID, attachmentID int, mutationErr error) error {
	if cleanupErr := s.uploads.DeleteUploadedPageAttachment(pageID, attachmentID); cleanupErr != nil {
		return errors.Join(mutationErr, fmt.Errorf("clean up failed page diagram attachment: %w", cleanupErr))
	}
	return mutationErr
}

func (s *PageDiagramService) latestRevisionNumber(pageID int) int {
	revisions, err := s.pages.PageService().ListRevisions(pageID, 1, 0)
	if err != nil || len(revisions) == 0 {
		return 0
	}
	return revisions[0].RevisionNumber
}

func parsePageDiagramBlocks(content string) []pageDiagramBlock {
	matches := pageDiagramFencePattern.FindAllStringSubmatchIndex(content, -1)
	blocks := make([]pageDiagramBlock, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		var payload struct {
			AttachmentID int    `json:"attachmentId"`
			Name         string `json:"name"`
		}
		if json.Unmarshal([]byte(content[match[2]:match[3]]), &payload) != nil ||
			payload.AttachmentID <= 0 {
			continue
		}
		blocks = append(blocks, pageDiagramBlock{
			start:        match[0],
			end:          match[1],
			attachmentID: payload.AttachmentID,
			name:         payload.Name,
		})
	}
	return blocks
}

func matchingPageDiagramBlocks(content string, attachmentID int) []pageDiagramBlock {
	var matches []pageDiagramBlock
	for _, block := range parsePageDiagramBlocks(content) {
		if block.attachmentID == attachmentID {
			matches = append(matches, block)
		}
	}
	return matches
}

func renderPageDiagramFence(attachmentID int, name string) string {
	payload, _ := json.Marshal(struct {
		AttachmentID int    `json:"attachmentId"`
		Name         string `json:"name"`
	}{AttachmentID: attachmentID, Name: name})
	return "```excalidraw\n" + string(payload) + "\n```"
}

func insertPageDiagramFence(content, fence, placement string) string {
	if strings.TrimSpace(content) == "" {
		return fence
	}
	if placement == PageDiagramPlacementStart {
		return fence + "\n\n" + content
	}
	separator := "\n\n"
	if strings.HasSuffix(content, "\n") {
		separator = "\n"
	}
	return content + separator + fence
}
