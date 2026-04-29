package scm

import (
	"context"
	"errors"
	"testing"
	"time"

	"windshift/internal/models"
)

// fakeProvider is a minimal Provider implementation used to drive
// pagination tests without any database or network dependency. Only the
// methods exercised by iteratePullRequests are populated; the rest panic
// to surface accidental calls.
type fakeProvider struct {
	pages       [][]PullRequest
	pageRequests []int // recorded Page values from each ListPullRequests call
	branches    []Branch
}

func (f *fakeProvider) GetType() models.SCMProviderType { return models.SCMProviderTypeGitHub }
func (f *fakeProvider) TestConnection(_ context.Context) error { return nil }
func (f *fakeProvider) ListRepositories(_ context.Context, _ ListRepositoriesOptions) ([]Repository, error) {
	panic("ListRepositories not implemented for fakeProvider")
}
func (f *fakeProvider) GetRepository(_ context.Context, _, _ string) (*Repository, error) {
	panic("GetRepository not implemented for fakeProvider")
}
func (f *fakeProvider) ListPullRequests(_ context.Context, _, _ string, opts ListPROptions) ([]PullRequest, error) {
	f.pageRequests = append(f.pageRequests, opts.Page)
	idx := opts.Page - 1
	if idx < 0 || idx >= len(f.pages) {
		return nil, nil
	}
	return f.pages[idx], nil
}
func (f *fakeProvider) GetPullRequest(_ context.Context, _, _ string, _ int) (*PullRequest, error) {
	panic("GetPullRequest not implemented for fakeProvider")
}
func (f *fakeProvider) ListPullRequestCommits(_ context.Context, _, _ string, _ int) ([]Commit, error) {
	panic("ListPullRequestCommits not implemented for fakeProvider")
}
func (f *fakeProvider) CreateBranch(_ context.Context, _, _, _, _ string) error {
	panic("CreateBranch not implemented for fakeProvider")
}
func (f *fakeProvider) CreatePullRequest(_ context.Context, _, _ string, _ CreatePROptions) (*PullRequest, error) {
	panic("CreatePullRequest not implemented for fakeProvider")
}
func (f *fakeProvider) GetCommit(_ context.Context, _, _, _ string) (*Commit, error) {
	panic("GetCommit not implemented for fakeProvider")
}
func (f *fakeProvider) ListBranches(_ context.Context, _, _ string) ([]Branch, error) {
	return f.branches, nil
}
func (f *fakeProvider) RegisterWebhook(_ context.Context, _, _ string, _ WebhookOptions) (*WebhookRegistration, error) {
	panic("RegisterWebhook not implemented for fakeProvider")
}
func (f *fakeProvider) DeleteWebhook(_ context.Context, _, _, _ string) error {
	panic("DeleteWebhook not implemented for fakeProvider")
}

// makePRs builds n PullRequests with UpdatedAt walking back one hour per
// step from start. Useful for exercising the cutoff branch.
func makePRs(n int, startNumber int, start time.Time) []PullRequest {
	out := make([]PullRequest, n)
	for i := 0; i < n; i++ {
		out[i] = PullRequest{
			Number:    startNumber + i,
			State:     "open",
			UpdatedAt: start.Add(-time.Duration(i) * time.Hour),
		}
	}
	return out
}

func TestIteratePullRequests_PaginatesUntilLastPage(t *testing.T) {
	now := time.Now()
	p := &fakeProvider{
		pages: [][]PullRequest{
			makePRs(syncPRsPerPage, 1, now),
			makePRs(syncPRsPerPage, syncPRsPerPage+1, now.Add(-100*time.Hour)),
			makePRs(50, 2*syncPRsPerPage+1, now.Add(-200*time.Hour)),
		},
	}

	var seen []int
	err := iteratePullRequests(context.Background(), p, "o", "r", time.Time{}, syncMaxPRs, func(pr PullRequest) {
		seen = append(seen, pr.Number)
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := len(seen), 2*syncPRsPerPage+50; got != want {
		t.Fatalf("processed %d PRs, want %d", got, want)
	}
	if got, want := len(p.pageRequests), 3; got != want {
		t.Fatalf("requested %d pages, want %d", got, want)
	}
}

func TestIteratePullRequests_StopsAtMax(t *testing.T) {
	now := time.Now()
	// Three full pages available, but max should cap us mid-page-2.
	p := &fakeProvider{
		pages: [][]PullRequest{
			makePRs(syncPRsPerPage, 1, now),
			makePRs(syncPRsPerPage, syncPRsPerPage+1, now.Add(-100*time.Hour)),
			makePRs(syncPRsPerPage, 2*syncPRsPerPage+1, now.Add(-200*time.Hour)),
		},
	}

	max := syncPRsPerPage + 50
	var seen []int
	err := iteratePullRequests(context.Background(), p, "o", "r", time.Time{}, max, func(pr PullRequest) {
		seen = append(seen, pr.Number)
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := len(seen); got != max {
		t.Fatalf("processed %d PRs, want %d", got, max)
	}
	// We should have requested exactly 2 pages (page 1 fills, page 2 partial then stop).
	if got, want := len(p.pageRequests), 2; got != want {
		t.Fatalf("requested %d pages, want %d", got, want)
	}
}

func TestIteratePullRequests_StopsAtLookbackCutoff(t *testing.T) {
	now := time.Now()
	lastSync := now.Add(-1 * time.Hour) // cutoff = lastSync - 7d

	// Page 1: PRs from now back to now-99h (all newer than cutoff).
	// Page 2: PRs from now-100h back to now-199h. Cutoff is at -169h
	// (lastSync minus 7*24h = -1h - 168h = -169h), so PRs at index 70+
	// on page 2 fall before cutoff.
	p := &fakeProvider{
		pages: [][]PullRequest{
			makePRs(syncPRsPerPage, 1, now),
			makePRs(syncPRsPerPage, syncPRsPerPage+1, now.Add(-100*time.Hour)),
			makePRs(syncPRsPerPage, 2*syncPRsPerPage+1, now.Add(-200*time.Hour)),
		},
	}

	var seen []int
	err := iteratePullRequests(context.Background(), p, "o", "r", lastSync, syncMaxPRs, func(pr PullRequest) {
		seen = append(seen, pr.Number)
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Cutoff = -169h. Page 1 entries span 0..-99h (all kept). Page 2
	// entries span -100h..-199h; the first PR at -169h or later passes,
	// the rest don't. Specifically: page2[0] = -100h, page2[68] = -168h,
	// page2[69] = -169h, page2[70] = -170h (first to fail). So 100 + 70
	// PRs survive; iteration stops before page 3 is requested.
	if got, want := len(seen), syncPRsPerPage+70; got != want {
		t.Fatalf("processed %d PRs, want %d (cutoff stop)", got, want)
	}
	if got, want := len(p.pageRequests), 2; got != want {
		t.Fatalf("requested %d pages, want %d", got, want)
	}
}

func TestIteratePullRequests_NoCutoffOnFirstSync(t *testing.T) {
	// lastSyncedAt zero (never synced) — no cutoff applies even for
	// PRs years old. Cap stops the walk.
	now := time.Now()
	old := now.Add(-365 * 24 * time.Hour)

	p := &fakeProvider{
		pages: [][]PullRequest{
			makePRs(syncPRsPerPage, 1, old),
			makePRs(syncPRsPerPage, syncPRsPerPage+1, old.Add(-100*time.Hour)),
		},
	}

	var seen []int
	err := iteratePullRequests(context.Background(), p, "o", "r", time.Time{}, syncMaxPRs, func(pr PullRequest) {
		seen = append(seen, pr.Number)
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := len(seen), 2*syncPRsPerPage; got != want {
		t.Fatalf("processed %d PRs, want %d (no cutoff)", got, want)
	}
}

func TestIteratePullRequests_PropagatesProviderError(t *testing.T) {
	pe := &errorProvider{err: errors.New("boom")}
	err := iteratePullRequests(context.Background(), pe, "o", "r", time.Time{}, syncMaxPRs, func(_ PullRequest) {})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// errorProvider always errors from ListPullRequests.
type errorProvider struct {
	fakeProvider
	err error
}

func (e *errorProvider) ListPullRequests(_ context.Context, _, _ string, _ ListPROptions) ([]PullRequest, error) {
	return nil, e.err
}
