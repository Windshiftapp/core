// Package notion provides an API client for the Notion integration.
package notion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiBaseURL     = "https://api.notion.com/v1"
	notionVersion  = "2022-06-28"
	requestTimeout = 10 * time.Second
)

// OAuthResult holds the result of a Notion OAuth token exchange.
type OAuthResult struct {
	AccessToken   string `json:"access_token"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	WorkspaceIcon string `json:"workspace_icon"`
	BotID         string `json:"bot_id"`
}

// NotionPage represents a page or database returned from the Notion API.
type NotionPage struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Icon     string `json:"icon"`
	PageType string `json:"page_type"` // "page" or "database"
}

// ExchangeOAuthCode exchanges an authorization code for an access token.
func ExchangeOAuthCode(clientID, clientSecret, code, redirectURI string) (*OAuthResult, error) {
	body := fmt.Sprintf(`{"grant_type":"authorization_code","code":%q,"redirect_uri":%q}`, code, redirectURI)
	req, err := http.NewRequest("POST", apiBaseURL+"/oauth/token", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion oauth error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var raw struct {
		AccessToken   string `json:"access_token"`
		WorkspaceID   string `json:"workspace_id"`
		WorkspaceName string `json:"workspace_name"`
		WorkspaceIcon string `json:"workspace_icon"`
		BotID         string `json:"bot_id"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &OAuthResult{
		AccessToken:   raw.AccessToken,
		WorkspaceID:   raw.WorkspaceID,
		WorkspaceName: raw.WorkspaceName,
		WorkspaceIcon: raw.WorkspaceIcon,
		BotID:         raw.BotID,
	}, nil
}

// SearchPages searches for pages and databases accessible to the integration.
func SearchPages(accessToken, query string) ([]NotionPage, error) {
	body := fmt.Sprintf(`{"query":%q,"filter":{"value":"page","property":"object"},"page_size":20}`, query)
	req, err := http.NewRequest("POST", apiBaseURL+"/search", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	setHeaders(req, accessToken)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching pages: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion search error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var searchResult struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(respBody, &searchResult); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Also search databases
	dbBody := fmt.Sprintf(`{"query":%q,"filter":{"value":"database","property":"object"},"page_size":10}`, query)
	dbReq, err := http.NewRequest("POST", apiBaseURL+"/search", strings.NewReader(dbBody))
	if err != nil {
		return nil, fmt.Errorf("creating database search request: %w", err)
	}
	setHeaders(dbReq, accessToken)

	dbResp, err := client.Do(dbReq)
	if err != nil {
		return nil, fmt.Errorf("searching databases: %w", err)
	}
	defer dbResp.Body.Close()

	dbRespBody, err := io.ReadAll(dbResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading database response: %w", err)
	}

	if dbResp.StatusCode == http.StatusOK {
		var dbResult struct {
			Results []json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(dbRespBody, &dbResult); err == nil {
			searchResult.Results = append(searchResult.Results, dbResult.Results...)
		}
	}

	pages := make([]NotionPage, 0, len(searchResult.Results))
	for _, raw := range searchResult.Results {
		page := parseNotionObject(raw)
		if page != nil {
			pages = append(pages, *page)
		}
	}

	return pages, nil
}

// GetPage retrieves a single page by ID.
func GetPage(accessToken, pageID string) (*NotionPage, error) {
	req, err := http.NewRequest("GET", apiBaseURL+"/pages/"+pageID, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	setHeaders(req, accessToken)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting page: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try as database
		return getDatabase(accessToken, pageID)
	}

	return parseNotionObject(respBody), nil
}

func getDatabase(accessToken, dbID string) (*NotionPage, error) {
	req, err := http.NewRequest("GET", apiBaseURL+"/databases/"+dbID, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	setHeaders(req, accessToken)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting database: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return parseNotionObject(respBody), nil
}

func setHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Notion-Version", notionVersion)
	req.Header.Set("Content-Type", "application/json")
}

func parseNotionObject(raw json.RawMessage) *NotionPage {
	var obj struct {
		ID     string `json:"id"`
		Object string `json:"object"`
		URL    string `json:"url"`
		Icon   *struct {
			Type     string `json:"type"`
			Emoji    string `json:"emoji"`
			External *struct {
				URL string `json:"url"`
			} `json:"external"`
		} `json:"icon"`
		Properties map[string]json.RawMessage `json:"properties"`
		Title      []struct {
			PlainText string `json:"plain_text"`
		} `json:"title"`
	}

	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	page := &NotionPage{
		ID:  obj.ID,
		URL: obj.URL,
	}

	// Determine type
	if obj.Object == "database" {
		page.PageType = "database"
	} else {
		page.PageType = "page"
	}

	// Extract icon
	if obj.Icon != nil {
		if obj.Icon.Type == "emoji" {
			page.Icon = obj.Icon.Emoji
		} else if obj.Icon.External != nil {
			page.Icon = obj.Icon.External.URL
		}
	}

	// Extract title - databases have a top-level "title" array
	if len(obj.Title) > 0 {
		page.Title = obj.Title[0].PlainText
	}

	// Pages have title in properties
	if page.Title == "" && obj.Properties != nil {
		page.Title = extractPageTitle(obj.Properties)
	}

	if page.Title == "" {
		page.Title = "Untitled"
	}

	return page
}

func extractPageTitle(properties map[string]json.RawMessage) string {
	// Look for a "title" type property
	for _, propRaw := range properties {
		var prop struct {
			Type  string `json:"type"`
			Title []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		}
		if err := json.Unmarshal(propRaw, &prop); err != nil {
			continue
		}
		if prop.Type == "title" && len(prop.Title) > 0 {
			return prop.Title[0].PlainText
		}
	}
	return ""
}
