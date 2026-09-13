package services

import (
	"context"
	"path/filepath"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
)

func newMilestoneReleaseTestService(t *testing.T) (*PlanningService, database.Database) {
	t.Helper()
	db, err := database.NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "windshift.db"), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Initialize(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return NewPlanningService(db), db
}

func insertGlobalMilestone(t *testing.T, db database.Database, name string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`INSERT INTO milestones(name, description, status, is_global) VALUES (?, '', 'planning', true) RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAttachReleaseStoresExtendedMetadata(t *testing.T) {
	service, db := newMilestoneReleaseTestService(t)
	defer func() { _ = db.Close() }()
	milestoneID := insertGlobalMilestone(t, db, "Attached release")
	tagURL := "https://git.example/group/project/-/tags/v1.0.0"
	releasedAt := "2026-08-30T12:00:00Z"

	err := service.AttachRelease(ReleaseMilestoneParams{
		ID: milestoneID, TagName: "v1.0.0", TagURL: &tagURL, ReleaseStatus: "released", ReleasedAt: &releasedAt,
		Assets: []models.SCMReleaseAsset{{Kind: "link", Name: "binary", URL: "https://cdn.example/app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	releases, err := service.ListMilestoneReleases(milestoneID)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].ReleaseStatus != "released" || len(releases[0].Assets) != 1 || releases[0].TagURL == nil || *releases[0].TagURL != tagURL {
		t.Fatalf("unexpected stored release: %+v", releases)
	}
}

func TestBeginAndCompleteMilestoneReleaseStoresProviderTruth(t *testing.T) {
	service, db := newMilestoneReleaseTestService(t)
	defer func() { _ = db.Close() }()
	milestoneID := insertGlobalMilestone(t, db, "Created release")

	params := ReleaseMilestoneParams{ID: milestoneID, IdempotencyKey: "release-key", TagName: "v2.0.0", Name: "requested name", Body: "requested body"}
	attempt, err := service.BeginMilestoneRelease(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	params.Name = "GitLab release"
	params.Body = "GitLab notes"
	params.ReleaseStatus = "released"
	params.Assets = []models.SCMReleaseAsset{{Kind: "source", Name: "zip", URL: "https://git.example/source.zip"}}
	result, err := service.CompleteMilestoneRelease(context.Background(), attempt.ID, attempt.LeaseToken, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.LatestRelease == nil || result.LatestRelease.Name != "GitLab release" || len(result.LatestRelease.Assets) != 1 {
		t.Fatalf("unexpected completed milestone: %+v", result)
	}
}
