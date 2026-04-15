package llm

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// decodeCompletionResponse reads an HTTP response and decodes it into a ChatCompletionResponse.
// It handles common error status codes (503 Service Unavailable, non-200) before attempting JSON decode.
func decodeCompletionResponse(resp *http.Response) (*ChatCompletionResponse, error) {
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrServiceNotReady
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return nil, fmt.Errorf("%w: status %d - %s", ErrAPIError, resp.StatusCode, string(respBody))
	}

	var result ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// scanConnections scans rows from an llm_connections query into a slice of ConnectionInfo.
// The rows must select: id, name, provider_type, model, api_key_encrypted, base_url, is_default, is_enabled, created_at, updated_at.
func scanConnections(rows *sql.Rows) ([]ConnectionInfo, error) {
	var connections []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		var apiKeyEncrypted, baseURL sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.ProviderType, &c.Model, &apiKeyEncrypted, &baseURL, &c.IsDefault, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}
		c.HasAPIKey = apiKeyEncrypted.Valid && apiKeyEncrypted.String != ""
		if baseURL.Valid {
			c.BaseURL = baseURL.String
		}
		connections = append(connections, c)
	}
	if connections == nil {
		connections = []ConnectionInfo{}
	}
	return connections, nil
}
