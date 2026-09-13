package scm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"windshift/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestGitLabProviderNestedProjectAndPAT(t *testing.T) {
	t.Parallel()
	provider, err := NewGitLabProvider(ProviderConfig{BaseURL: "https://git.example/api/v4", AuthMethod: models.SCMAuthMethodPAT, PersonalAccessToken: "pat-token"})
	if err != nil {
		t.Fatal(err)
	}
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.EscapedPath() != "/api/v4/projects/group%2Fsubgroup%2Fproject" {
			t.Fatalf("unexpected path: %s", request.URL.EscapedPath())
		}
		if got := request.Header.Get("PRIVATE-TOKEN"); got != "pat-token" {
			t.Fatalf("unexpected PAT header: %q", got)
		}
		return jsonResponse(http.StatusOK, `{
			"id":42,"name":"project","path_with_namespace":"group/subgroup/project",
			"web_url":"https://git.example/group/subgroup/project",
			"http_url_to_repo":"https://git.example/group/subgroup/project.git",
			"ssh_url_to_repo":"git@git.example:group/subgroup/project.git",
			"default_branch":"main","visibility":"private",
			"namespace":{"full_path":"group/subgroup"}
		}`), nil
	})}

	repository, err := provider.GetRepository(context.Background(), "group/subgroup", "project")
	if err != nil {
		t.Fatal(err)
	}
	if repository.FullName != "group/subgroup/project" || repository.Owner != "group/subgroup" || repository.ID != "42" {
		t.Fatalf("unexpected repository mapping: %+v", repository)
	}
}

func TestGitLabProviderReleaseMetadata(t *testing.T) {
	t.Parallel()
	const releasedAt = "2026-08-30T12:00:00Z"
	provider, err := NewGitLabProvider(ProviderConfig{BaseURL: "https://git.example", AuthMethod: models.SCMAuthMethodOAuth, OAuthAccessToken: "oauth-token"})
	if err != nil {
		t.Fatal(err)
	}
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer oauth-token" {
			t.Fatalf("unexpected OAuth header: %q", got)
		}
		return jsonResponse(http.StatusOK, `[{
			"tag_name":"v2.0.0","name":"Version 2","description":"Notes",
			"created_at":"2026-08-30T12:00:00Z","released_at":"2026-08-30T12:00:00Z",
			"_links":{"self":"https://git.example/group/project/-/releases/v2.0.0"},
			"assets":{
				"sources":[{"format":"zip","url":"https://git.example/source.zip"}],
				"links":[{"name":"binary","url":"https://cdn.example/app","direct_asset_url":"/downloads/app","link_type":"package"}]
			}
		}]`), nil
	})}

	releases, err := provider.ListReleases(context.Background(), "group", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].Status != "released" || releases[0].TagURL != "https://git.example/group/project/-/tags/v2.0.0" {
		t.Fatalf("unexpected release mapping: %+v", releases)
	}
	if len(releases[0].Assets) != 2 || releases[0].Assets[1].DirectURL != "/downloads/app" {
		t.Fatalf("unexpected assets: %+v", releases[0].Assets)
	}
	want, _ := time.Parse(time.RFC3339, releasedAt)
	if releases[0].ReleasedAt == nil || !releases[0].ReleasedAt.Equal(want) {
		t.Fatalf("unexpected release date: %v", releases[0].ReleasedAt)
	}
}

func TestGitLabProviderRefreshToken(t *testing.T) {
	t.Parallel()
	provider, err := NewGitLabProvider(ProviderConfig{BaseURL: "https://git.example", AuthMethod: models.SCMAuthMethodOAuth, OAuthClientID: "client", OAuthClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/oauth/token" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		values := string(body)
		if !strings.Contains(values, "grant_type=refresh_token") || !strings.Contains(values, "refresh_token=refresh-me") {
			t.Fatalf("unexpected form: %s", values)
		}
		return jsonResponse(http.StatusOK, `{"access_token":"new-token","refresh_token":"next-refresh","token_type":"Bearer","expires_in":7200,"scope":"api"}`), nil
	})}

	tokens, err := provider.RefreshToken(context.Background(), "refresh-me")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "new-token" || tokens.RefreshToken != "next-refresh" || tokens.ExpiresAt == nil {
		t.Fatalf("unexpected tokens: %+v", tokens)
	}
}
