//go:build !noplugins

package plugins

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	extism "github.com/extism/go-sdk"
)

// buildHostFunctions creates all host functions available to plugins.
func (m *Manager) buildHostFunctions() []extism.HostFunction {
	return []extism.HostFunction{
		extism.NewHostFunctionWithStack("log", m.logHostFunction, []extism.ValueType{extism.ValueTypeI64}, nil),
		extism.NewHostFunctionWithStack("smtp_send", m.smtpHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("http_fetch", m.httpFetchHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("cli_exec", m.cliExecHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_get", m.kvGetHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_set", m.kvSetHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_delete", m.kvDeleteHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("create_comment", m.createCommentHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("scm_create_branch", m.scmCreateBranchHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("scm_create_item_link", m.scmCreateItemLinkHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
	}
}

func (m *Manager) logHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("log host function failed to read payload", "error", err)
		return
	}

	var logReq LogRequest
	if err := json.Unmarshal(payload, &logReq); err != nil {
		m.logger.Warn("log host function failed to parse payload", "error", err)
		return
	}

	level := slog.LevelInfo
	switch strings.ToLower(logReq.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		level = slog.LevelInfo
	}

	m.logger.Log(ctx, level, logReq.Message)
}

func (m *Manager) smtpHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("smtp_send host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var sendReq SMTPSendRequest
	if err := json.Unmarshal(payload, &sendReq); err != nil {
		m.logger.Warn("smtp_send host function failed to parse payload", "error", err)
		stack[0] = 0
		return
	}

	result := SMTPSendResponse{Status: "ok"}
	if m.smtpSender == nil {
		result.Status = "error"
		result.Error = "smtp sender not configured"
	} else if err := m.smtpSender.Send(ctx, sendReq); err != nil {
		result.Status = "error"
		result.Error = err.Error()
	}

	m.writeHostResponse(plugin, stack, result)
}

func (m *Manager) httpFetchHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("http_fetch host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var fetchReq HTTPFetchRequest
	if err := json.Unmarshal(payload, &fetchReq); err != nil {
		m.logger.Warn("http_fetch host function failed to parse payload", "error", err)
		stack[0] = 0
		return
	}

	method := strings.ToUpper(fetchReq.Method)
	if method == "" {
		method = http.MethodGet
	}

	if fetchReq.URL == "" {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadRequest})
		return
	}

	client := m.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	timeout := m.pluginTimeout
	if fetchReq.TimeoutMs > 0 {
		timeout = time.Duration(fetchReq.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, fetchReq.URL, bytes.NewReader(fetchReq.Body))
	if err != nil {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadRequest, Body: []byte(err.Error())})
		return
	}

	for k, v := range fetchReq.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadGateway, Body: []byte(err.Error())})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	m.writeHostResponse(plugin, stack, HTTPFetchResponse{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    body,
	})
}

func (m *Manager) writeHostResponse(plugin *extism.CurrentPlugin, stack []uint64, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("host response marshal failed", "error", err)
		stack[0] = 0
		return
	}

	ptr, err := plugin.WriteBytes(data)
	if err != nil {
		m.logger.Warn("host response write failed", "error", err)
		stack[0] = 0
		return
	}

	stack[0] = ptr
}

func (m *Manager) kvGetHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_get host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVGetRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_get host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "database not configured"})
		return
	}

	var value string
	err = m.db.QueryRowContext(ctx,
		"SELECT value FROM plugin_kv_store WHERE plugin_name = ? AND key = ?",
		pluginName, kvReq.Key,
	).Scan(&value)

	if err == sql.ErrNoRows {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "not_found"})
		return
	}
	if err != nil {
		m.logger.Warn("kv_get database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVGetResponse{Status: "ok", Value: value})
}

func (m *Manager) kvSetHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_set host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVSetRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_set host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "database not configured"})
		return
	}

	now := time.Now()
	// Use upsert pattern - INSERT ... ON CONFLICT UPDATE
	var query string
	if m.db.GetDriverName() == "postgres" {
		query = `
			INSERT INTO plugin_kv_store (plugin_name, key, value, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (plugin_name, key) DO UPDATE SET value = $3, updated_at = $5
		`
	} else {
		query = `
			INSERT INTO plugin_kv_store (plugin_name, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(plugin_name, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`
	}

	_, err = m.db.ExecWriteContext(ctx, query, pluginName, kvReq.Key, kvReq.Value, now, now)
	if err != nil {
		m.logger.Warn("kv_set database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVSetResponse{Status: "ok"})
}

func (m *Manager) kvDeleteHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_delete host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVDeleteRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_delete host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "database not configured"})
		return
	}

	_, err = m.db.ExecWriteContext(ctx,
		"DELETE FROM plugin_kv_store WHERE plugin_name = ? AND key = ?",
		pluginName, kvReq.Key,
	)
	if err != nil {
		m.logger.Warn("kv_delete database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "ok"})
}

func (m *Manager) createCommentHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("create_comment host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req CreateCommentRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("create_comment host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.ItemID <= 0 {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "item_id is required"})
		return
	}
	if req.AuthorID <= 0 {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "author_id is required"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "content is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "database not configured"})
		return
	}

	// Verify item exists
	var itemExists bool
	err = m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", req.ItemID).Scan(&itemExists)
	if err != nil || !itemExists {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "item not found"})
		return
	}

	// Verify author exists
	var authorExists bool
	err = m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", req.AuthorID).Scan(&authorExists)
	if err != nil || !authorExists {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "author not found"})
		return
	}

	// Convert plain text to TipTap JSON format
	content := convertToTipTapJSON(req.Content)

	// Insert the comment
	now := time.Now()
	var commentID int64
	err = m.db.QueryRowContext(ctx, `
		INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, req.ItemID, req.AuthorID, content, now, now).Scan(&commentID)
	if err != nil {
		m.logger.Warn("create_comment database error", "error", err, "plugin", pluginName, "item_id", req.ItemID)
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "failed to create comment"})
		return
	}

	m.logger.Info("plugin created comment", "plugin", pluginName, "comment_id", commentID, "item_id", req.ItemID)
	m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "ok", CommentID: int(commentID)})
}

func (m *Manager) scmCreateBranchHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("scm_create_branch host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req SCMCreateBranchRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("scm_create_branch host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.WorkspaceRepositoryID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "workspace_repository_id is required"})
		return
	}
	if strings.TrimSpace(req.BranchName) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "branch_name is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.scmService == nil {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "SCM service not configured"})
		return
	}

	branchURL, err := m.scmService.CreateBranchForRepository(ctx, req.WorkspaceRepositoryID, req.BranchName, req.BaseBranch)
	if err != nil {
		m.logger.Warn("scm_create_branch failed", "error", err, "plugin", pluginName, "repo_id", req.WorkspaceRepositoryID)
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: err.Error()})
		return
	}

	m.logger.Info("plugin created branch", "plugin", pluginName, "repo_id", req.WorkspaceRepositoryID, "branch", req.BranchName)
	m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "ok", BranchURL: branchURL})
}

func (m *Manager) scmCreateItemLinkHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("scm_create_item_link host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req SCMCreateItemLinkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("scm_create_item_link host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.ItemID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "item_id is required"})
		return
	}
	if req.WorkspaceRepositoryID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "workspace_repository_id is required"})
		return
	}
	if strings.TrimSpace(req.LinkType) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "link_type is required"})
		return
	}
	if strings.TrimSpace(req.ExternalID) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "external_id is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.scmService == nil {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "SCM service not configured"})
		return
	}

	linkID, err := m.scmService.CreateItemSCMLink(ctx, req.ItemID, req.WorkspaceRepositoryID, req.LinkType, req.ExternalID, req.ExternalURL, req.Title)
	if err != nil {
		m.logger.Warn("scm_create_item_link failed", "error", err, "plugin", pluginName, "item_id", req.ItemID)
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: err.Error()})
		return
	}

	m.logger.Info("plugin created item SCM link", "plugin", pluginName, "item_id", req.ItemID, "link_id", linkID)
	m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "ok", LinkID: linkID})
}

// convertToTipTapJSON converts plain text to TipTap JSON format for rich text storage.
func convertToTipTapJSON(plainText string) string {
	// Split by newlines to create paragraphs
	lines := strings.Split(plainText, "\n")
	paragraphs := make([]map[string]interface{}, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			// Empty line becomes empty paragraph
			paragraphs = append(paragraphs, map[string]interface{}{
				"type": "paragraph",
			})
		} else {
			paragraphs = append(paragraphs, map[string]interface{}{
				"type": "paragraph",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": line,
					},
				},
			})
		}
	}

	doc := map[string]interface{}{
		"type":    "doc",
		"content": paragraphs,
	}

	jsonBytes, _ := json.Marshal(doc)
	return string(jsonBytes)
}
