// Package xray provides read-only clients for importing Xray test definitions.
package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"windshift/internal/utils"
)

const (
	maxCloudTestsPerRequest = 100
	maxCloudRateRetries     = 2
	maxCloudRetryDelay      = 30 * time.Second
)

var cloudRegionHosts = map[string]string{
	"global": "xray.cloud.getxray.app",
	"us":     "us.xray.cloud.getxray.app",
	"eu":     "eu.xray.cloud.getxray.app",
	"au":     "au.xray.cloud.getxray.app",
}

// Step is the portable manual-step representation shared by Cloud GraphQL and
// Data Center Raven responses.
type Step struct {
	Action   string
	Data     string
	Expected string
}

// Test is an Xray test definition keyed by Jira's numeric issue ID.
type Test struct {
	IssueID      string
	TestTypeName string
	Steps        []Step
}

// DefinitionClient loads Xray definitions for Jira numeric issue IDs.
type DefinitionClient interface {
	Validate(ctx context.Context) error
	GetTests(ctx context.Context, issueIDs []string) ([]Test, error)
}

// CloudConfig configures the Xray Cloud API. Region must be one of global, us,
// eu, or au; arbitrary hosts are intentionally unsupported.
type CloudConfig struct {
	ClientID     string
	ClientSecret string
	Region       string
}

// NewCloudClient constructs a Cloud definition client with a fixed regional
// Xray endpoint.
func NewCloudClient(config CloudConfig) (DefinitionClient, error) {
	clientID := strings.TrimSpace(config.ClientID)
	clientSecret := strings.TrimSpace(config.ClientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("xray client ID and client secret are required")
	}
	region := strings.ToLower(strings.TrimSpace(config.Region))
	if region == "" {
		region = "global"
	}
	host, ok := cloudRegionHosts[region]
	if !ok {
		return nil, fmt.Errorf("unsupported Xray Cloud region %q", config.Region)
	}

	timeout := 30 * time.Second
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: utils.NewSSRFSafeTransport(timeout),
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Hostname(), host) {
				return errors.New("xray redirect changed origin")
			}
			return nil
		},
	}
	return newCloudClientWithBaseURL("https://"+host, clientID, clientSecret, httpClient), nil
}

type cloudClient struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	wait         func(context.Context, time.Duration) error

	tokenMu sync.Mutex
	token   string
}

func newCloudClientWithBaseURL(baseURL, clientID, clientSecret string, httpClient *http.Client) *cloudClient {
	return &cloudClient{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
		wait: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type cloudTestType struct {
	Name string `json:"name"`
}

type cloudStep struct {
	Action string `json:"action"`
	Data   string `json:"data"`
	Result string `json:"result"`
}

type cloudTest struct {
	IssueID  string        `json:"issueId"`
	TestType cloudTestType `json:"testType"`
	Steps    []cloudStep   `json:"steps"`
}

type graphQLResponse struct {
	Data struct {
		GetTests struct {
			Results []cloudTest `json:"results"`
		} `json:"getTests"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

const getTestsQuery = `
query ImportTests($issueIds: [String], $limit: Int!) {
  getTests(issueIds: $issueIds, limit: $limit) {
    results {
      issueId
      testType { name }
      steps {
        action
        data
        result
      }
    }
  }
}`

// GetTests authenticates lazily and fetches definitions in Xray's documented
// maximum batches of 100.
func (c *cloudClient) GetTests(ctx context.Context, issueIDs []string) ([]Test, error) {
	if len(issueIDs) == 0 {
		return []Test{}, nil
	}
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, err
	}

	tests := make([]Test, 0, len(issueIDs))
	for start := 0; start < len(issueIDs); start += maxCloudTestsPerRequest {
		end := start + maxCloudTestsPerRequest
		if end > len(issueIDs) {
			end = len(issueIDs)
		}
		batch, err := c.getTestsBatch(ctx, token, issueIDs[start:end])
		if err != nil {
			return nil, err
		}
		tests = append(tests, batch...)
	}
	return tests, nil
}

// Validate authenticates without retaining the long-lived client secret
// anywhere outside this in-memory client.
func (c *cloudClient) Validate(ctx context.Context) error {
	_, err := c.authenticate(ctx)
	return err
}

func (c *cloudClient) authenticate(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" {
		return c.token, nil
	}

	payload, err := json.Marshal(map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	})
	if err != nil {
		return "", err
	}
	resp, err := c.postWithRateLimitRetry(ctx, "/api/v2/authenticate", payload, "")
	if err != nil {
		return "", fmt.Errorf("authenticate with Xray Cloud: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", xrayHTTPError("authenticate with Xray Cloud", resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read Xray authentication response: %w", err)
	}
	var token string
	if err := json.Unmarshal(body, &token); err != nil {
		var envelope struct {
			Token string `json:"token"`
		}
		if envelopeErr := json.Unmarshal(body, &envelope); envelopeErr != nil {
			return "", fmt.Errorf("decode Xray authentication response: %w", err)
		}
		token = envelope.Token
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("xray authentication returned an empty token")
	}
	c.token = token
	return c.token, nil
}

func (c *cloudClient) getTestsBatch(ctx context.Context, token string, issueIDs []string) ([]Test, error) {
	payload, err := json.Marshal(graphQLRequest{
		Query: getTestsQuery,
		Variables: map[string]any{
			"issueIds": issueIDs,
			"limit":    len(issueIDs),
		},
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.postWithRateLimitRetry(ctx, "/api/v2/graphql", payload, token)
	if err != nil {
		return nil, fmt.Errorf("query Xray Cloud tests: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, xrayHTTPError("query Xray Cloud tests", resp)
	}

	var response graphQLResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode Xray GraphQL response: %w", err)
	}
	if len(response.Errors) > 0 {
		messages := make([]string, 0, len(response.Errors))
		for _, graphErr := range response.Errors {
			if graphErr.Message != "" {
				messages = append(messages, graphErr.Message)
			}
		}
		return nil, fmt.Errorf("xray GraphQL returned errors: %s", strings.Join(messages, "; "))
	}

	tests := make([]Test, 0, len(response.Data.GetTests.Results))
	for _, source := range response.Data.GetTests.Results {
		test := Test{
			IssueID:      source.IssueID,
			TestTypeName: source.TestType.Name,
			Steps:        make([]Step, 0, len(source.Steps)),
		}
		for _, step := range source.Steps {
			test.Steps = append(test.Steps, Step{
				Action:   step.Action,
				Data:     step.Data,
				Expected: step.Result,
			})
		}
		tests = append(tests, test)
	}
	return tests, nil
}

func (c *cloudClient) postWithRateLimitRetry(
	ctx context.Context,
	path string,
	payload []byte,
	token string,
) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.baseURL+path,
			bytes.NewReader(payload),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= maxCloudRateRetries {
			return resp, nil
		}

		delay := xrayRetryDelay(resp.Header.Get("Retry-After"), time.Now())
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
		if err := c.wait(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func xrayRetryDelay(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		delay := time.Duration(seconds) * time.Second
		if delay > maxCloudRetryDelay {
			return maxCloudRetryDelay
		}
		return delay
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		delay := retryAt.Sub(now)
		if delay < 0 {
			return 0
		}
		if delay > maxCloudRetryDelay {
			return maxCloudRetryDelay
		}
		return delay
	}
	return time.Second
}

func xrayHTTPError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("%s: status %d: %s", operation, resp.StatusCode, message)
}
