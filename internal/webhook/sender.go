// Package webhook provides webhook delivery and management functionality.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/utils"
)

// newSSRFSafeWebhookClient returns an http.Client whose transport refuses
// to dial loopback / RFC1918 / link-local / CGNAT addresses. Used for both
// the long-lived production webhook client and the per-test client so the
// validate-then-dial gap (DNS rebinding between ValidateWebhookURL and the
// actual HTTP request) is closed at the dialer.
func newSSRFSafeWebhookClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Never forward configured authentication headers or request bodies
			// to a receiver-selected destination. Treat every 3xx as a failed
			// delivery and require operators to configure the final URL.
			return http.ErrUseLastResponse
		},
		Transport: utils.NewSSRFSafeTransport(timeout),
	}
}

// PluginDispatcher is an interface for dispatching webhooks to plugins
type PluginDispatcher interface {
	DispatchToPlugin(ctx context.Context, pluginName, handler, event string, payload json.RawMessage) error
}

const (
	// dispatchWorkerCount bounds simultaneous event matching and delivery work.
	dispatchWorkerCount = 8
	// dispatchQueueCapacity bounds events waiting for a worker. Automatic
	// webhook delivery is best-effort: when this queue is full, a new event is
	// rejected and counted instead of creating another goroutine or SQL waiter.
	dispatchQueueCapacity = 256
	// subscriptionCacheTTL bounds stale configuration when another replica or
	// a plugin mutates channels without reaching this process's invalidator.
	subscriptionCacheTTL = 30 * time.Second
	// destinationConcurrency prevents one receiver from consuming every event
	// worker while still allowing modest parallelism per channel.
	destinationConcurrency = 2
)

var supportedAutomaticEvents = map[string]bool{
	"item.created":   true,
	"item.updated":   true,
	"item.deleted":   true,
	"item.assigned":  true,
	"status.changed": true,
}

type dispatchJob struct {
	event string
	item  models.Item
}

// DispatchStats is a snapshot of the bounded automatic webhook pipeline.
type DispatchStats struct {
	QueueDepth                int     `json:"queue_depth"`
	QueueCapacity             int     `json:"queue_capacity"`
	OldestEventAgeMillis      int64   `json:"oldest_event_age_ms"`
	ActiveWorkers             int64   `json:"active_workers"`
	Enqueued                  uint64  `json:"enqueued"`
	Processed                 uint64  `json:"processed"`
	Rejected                  uint64  `json:"rejected"`
	Dropped                   uint64  `json:"dropped"`
	Retried                   uint64  `json:"retried"`
	FailedEvents              uint64  `json:"failed_events"`
	SubscriptionCacheEntries  int     `json:"subscription_cache_entries"`
	SubscriptionCacheHits     uint64  `json:"subscription_cache_hits"`
	SubscriptionCacheMisses   uint64  `json:"subscription_cache_misses"`
	SubscriptionInvalidations uint64  `json:"subscription_invalidations"`
	DeliveryCount             uint64  `json:"delivery_count"`
	AverageDeliveryLatencyMs  float64 `json:"average_delivery_latency_ms"`
	MaximumDeliveryLatencyMs  int64   `json:"maximum_delivery_latency_ms"`
	DatabaseTimeMillis        int64   `json:"database_time_ms"`
}

type subscriptionIndex struct {
	byEvent   map[string][]WebhookConfig
	entries   int
	expiresAt time.Time
}

// WebhookSender handles sending webhooks to configured endpoints
type WebhookSender struct {
	db               database.Database
	itemRepository   *repository.ItemRepository
	deliveryRepo     *repository.WebhookDeliveryRepository
	httpClient       *http.Client
	pluginDispatcher PluginDispatcher
	encryption       email.Encryptor

	dispatchCtx    context.Context
	dispatchCancel context.CancelFunc
	dispatchQueue  chan dispatchJob
	dispatchJobFn  func(dispatchJob)
	itemPayloadFn  func(context.Context, string, *models.Item) (json.RawMessage, error)
	dispatchMu     sync.RWMutex
	accepting      bool
	dispatchWG     sync.WaitGroup
	activeWorkers  atomic.Int64
	enqueued       atomic.Uint64
	rejected       atomic.Uint64
	processed      atomic.Uint64
	retried        atomic.Uint64
	failedEvents   atomic.Uint64
	oldestEventNS  atomic.Int64
	databaseNanos  atomic.Int64
	deliveryCount  atomic.Uint64
	deliveryNanos  atomic.Int64
	maxDeliveryNS  atomic.Int64

	subscriptionMu            sync.RWMutex
	subscriptions             *subscriptionIndex
	subscriptionHits          atomic.Uint64
	subscriptionMisses        atomic.Uint64
	subscriptionInvalidations atomic.Uint64

	destinationMu     sync.Mutex
	destinationLimits map[int]chan struct{}
}

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(db database.Database, encryptors ...email.Encryptor) *WebhookSender {
	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	w := &WebhookSender{
		db:                db,
		itemRepository:    repository.NewItemRepository(db),
		deliveryRepo:      repository.NewWebhookDeliveryRepository(db),
		httpClient:        newSSRFSafeWebhookClient(30 * time.Second),
		dispatchCtx:       dispatchCtx,
		dispatchCancel:    dispatchCancel,
		dispatchQueue:     make(chan dispatchJob, dispatchQueueCapacity),
		accepting:         true,
		destinationLimits: make(map[int]chan struct{}),
	}
	if len(encryptors) > 0 {
		w.encryption = encryptors[0]
	}
	w.dispatchJobFn = w.dispatchQueuedEvent
	w.itemPayloadFn = w.itemPayloadJSON
	w.startDispatchWorkers(dispatchWorkerCount)
	return w
}

func (w *WebhookSender) startDispatchWorkers(workerCount int) {
	w.dispatchWG.Add(workerCount)
	for range workerCount {
		go func() {
			defer w.dispatchWG.Done()
			for job := range w.dispatchQueue {
				w.activeWorkers.Add(1)
				w.dispatchJobFn(job)
				w.processed.Add(1)
				remainingWorkers := w.activeWorkers.Add(-1)
				if len(w.dispatchQueue) == 0 && remainingWorkers == 0 {
					w.oldestEventNS.Store(0)
				}
			}
		}()
	}
}

func (w *WebhookSender) dispatchQueuedEvent(job dispatchJob) {
	ctx, cancel := context.WithTimeout(w.dispatchCtx, 60*time.Second)
	defer cancel()

	dbStart := time.Now()
	webhooks, err := w.GetMatchingWebhooks(ctx, job.event, &job.item)
	w.databaseNanos.Add(time.Since(dbStart).Nanoseconds())
	if err != nil {
		w.failedEvents.Add(1)
		logger.Get().Error("Failed to get matching webhooks", "error", err, "event", job.event, "item_id", job.item.ID)
		return
	}
	if len(webhooks) == 0 {
		return
	}

	dbStart = time.Now()
	payloadFn := w.itemPayloadFn
	if payloadFn == nil {
		payloadFn = w.itemPayloadJSON
	}
	itemJSON, err := payloadFn(ctx, job.event, &job.item)
	w.databaseNanos.Add(time.Since(dbStart).Nanoseconds())
	if err != nil {
		w.failedEvents.Add(1)
		logger.Get().Error("Failed to hydrate webhook item payload", "error", err, "event", job.event, "item_id", job.item.ID)
		for _, webhook := range webhooks {
			w.recordPayloadFailure(ctx, webhook, job.event, job.item.ID, err)
		}
		return
	}
	for _, webhook := range webhooks {
		if err := w.sendWebhookPayload(ctx, webhook, job.event, job.item.ID, itemJSON); err != nil {
			logger.Get().Warn("Webhook delivery failed", "error", err, "event", job.event, "item_id", job.item.ID, "channel_id", webhook.ChannelID)
		}
	}
}

// Stats returns current queue pressure and lifetime admission counters.
func (w *WebhookSender) Stats() DispatchStats {
	oldestAge := int64(0)
	if oldest := w.oldestEventNS.Load(); oldest > 0 {
		oldestAge = time.Since(time.Unix(0, oldest)).Milliseconds()
		if oldestAge < 0 {
			oldestAge = 0
		}
	}
	w.subscriptionMu.RLock()
	cacheEntries := 0
	if w.subscriptions != nil {
		cacheEntries = w.subscriptions.entries
	}
	w.subscriptionMu.RUnlock()
	deliveryCount := w.deliveryCount.Load()
	averageDeliveryMillis := 0.0
	if deliveryCount > 0 {
		averageDeliveryMillis = float64(w.deliveryNanos.Load()) / float64(deliveryCount) / float64(time.Millisecond)
	}
	rejected := w.rejected.Load()
	return DispatchStats{
		QueueDepth:                len(w.dispatchQueue),
		QueueCapacity:             cap(w.dispatchQueue),
		OldestEventAgeMillis:      oldestAge,
		ActiveWorkers:             w.activeWorkers.Load(),
		Enqueued:                  w.enqueued.Load(),
		Processed:                 w.processed.Load(),
		Rejected:                  rejected,
		Dropped:                   rejected,
		Retried:                   w.retried.Load(),
		FailedEvents:              w.failedEvents.Load(),
		SubscriptionCacheEntries:  cacheEntries,
		SubscriptionCacheHits:     w.subscriptionHits.Load(),
		SubscriptionCacheMisses:   w.subscriptionMisses.Load(),
		SubscriptionInvalidations: w.subscriptionInvalidations.Load(),
		DeliveryCount:             deliveryCount,
		AverageDeliveryLatencyMs:  averageDeliveryMillis,
		MaximumDeliveryLatencyMs:  w.maxDeliveryNS.Load() / int64(time.Millisecond),
		DatabaseTimeMillis:        w.databaseNanos.Load() / int64(time.Millisecond),
	}
}

// Shutdown stops admission and drains queued automatic events. If ctx expires,
// in-flight HTTP/plugin requests are canceled and the undrained count is
// returned to the caller.
func (w *WebhookSender) Shutdown(ctx context.Context) error {
	w.dispatchMu.Lock()
	if w.accepting {
		w.accepting = false
		close(w.dispatchQueue)
	}
	w.dispatchMu.Unlock()

	done := make(chan struct{})
	go func() {
		w.dispatchWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.dispatchCancel()
		return nil
	case <-ctx.Done():
		queued := len(w.dispatchQueue)
		w.dispatchCancel()
		return fmt.Errorf("webhook dispatch shutdown with %d queued events: %w", queued, ctx.Err())
	}
}

// recordDelivery persists a delivery row. Failures here are logged but never
// propagated — recording must not block actual webhook dispatch.
func (w *WebhookSender) recordDelivery(ctx context.Context, d *models.WebhookDelivery) {
	start := time.Now()
	defer func() { w.databaseNanos.Add(time.Since(start).Nanoseconds()) }()
	recordCtx := ctx
	cancel := func() {}
	if ctx == nil || ctx.Err() != nil {
		recordCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	}
	defer cancel()
	if err := w.deliveryRepo.Insert(recordCtx, d); err != nil {
		logger.Get().Warn("Failed to record webhook delivery", "error", err, "channel_id", d.ChannelID)
	}
}

// attemptTypeFor returns "manual" for the literal "manual" event (TriggerManually
// passes that value), "automatic" otherwise.
func attemptTypeFor(event string) string {
	if event == "manual" {
		return "manual"
	}
	return "automatic"
}

// SetPluginDispatcher sets the plugin dispatcher for handling plugin webhooks
func (w *WebhookSender) SetPluginDispatcher(dispatcher PluginDispatcher) {
	w.pluginDispatcher = dispatcher
}

// WebhookPayload represents the payload sent to webhook endpoints
type WebhookPayload struct {
	Event     string          `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	WebhookID int             `json:"webhook_id"`
	Item      json.RawMessage `json:"item"`
}

// WebhookConfig represents a webhook configuration from the channels table
type WebhookConfig struct {
	ChannelID        int
	Name             string
	URL              string
	Secret           string
	Headers          map[string]string
	ScopeType        string // "all", "workspaces", "collections"
	WorkspaceIDs     []int
	CollectionIDs    []int
	AutoTrigger      bool
	SubscribedEvents []string
	// Plugin webhook fields
	PluginName      string // Non-empty if this is a plugin webhook
	PluginWebhookID string // Plugin's webhook identifier
	PluginHandler   string // Plugin's handler function name
}

// DispatchEvent admits an automatic webhook event without blocking on SQL or
// downstream delivery. The queue is deliberately lossy when full so request
// mutation paths stay bounded; rejected events are logged and counted.
func (w *WebhookSender) DispatchEvent(event string, item *models.Item) {
	if item == nil {
		return
	}

	now := time.Now()
	job := dispatchJob{event: event, item: *item}
	w.dispatchMu.RLock()
	defer w.dispatchMu.RUnlock()
	if !w.accepting {
		w.rejected.Add(1)
		return
	}

	select {
	case w.dispatchQueue <- job:
		w.enqueued.Add(1)
		w.oldestEventNS.CompareAndSwap(0, now.UnixNano())
	default:
		w.rejected.Add(1)
		logger.Get().Warn("Webhook event queue full; event rejected",
			"event", event,
			"item_id", item.ID,
			"queue_capacity", cap(w.dispatchQueue),
			"rejected_total", w.rejected.Load(),
		)
	}
}

// GetMatchingWebhooks returns all webhooks that should fire for this event and item
func (w *WebhookSender) GetMatchingWebhooks(ctx context.Context, event string, item *models.Item) ([]WebhookConfig, error) {
	index, err := w.subscriptionIndex(ctx)
	if err != nil {
		return nil, err
	}
	candidates := index.byEvent[event]
	matching := make([]WebhookConfig, 0, len(candidates))
	for _, webhook := range candidates {
		config := models.ChannelConfig{
			WebhookScopeType:     webhook.ScopeType,
			WebhookWorkspaceIDs:  webhook.WorkspaceIDs,
			WebhookCollectionIDs: webhook.CollectionIDs,
		}
		if w.matchesScope(ctx, &config, item) {
			matching = append(matching, webhook)
		}
	}
	return matching, nil
}

// InvalidateSubscriptions makes local channel mutations visible to the next
// event immediately. The TTL covers mutations performed by other replicas or
// plugin lifecycle code that does not share this sender instance.
func (w *WebhookSender) InvalidateSubscriptions() {
	w.subscriptionMu.Lock()
	w.subscriptions = nil
	w.subscriptionMu.Unlock()
	w.subscriptionInvalidations.Add(1)
}

func (w *WebhookSender) subscriptionIndex(ctx context.Context) (*subscriptionIndex, error) {
	now := time.Now()
	w.subscriptionMu.RLock()
	current := w.subscriptions
	if current != nil && now.Before(current.expiresAt) {
		w.subscriptionMu.RUnlock()
		w.subscriptionHits.Add(1)
		return current, nil
	}
	w.subscriptionMu.RUnlock()

	w.subscriptionMu.Lock()
	defer w.subscriptionMu.Unlock()
	if current = w.subscriptions; current != nil && time.Now().Before(current.expiresAt) {
		w.subscriptionHits.Add(1)
		return current, nil
	}
	w.subscriptionMisses.Add(1)

	query := `
		SELECT id, name, COALESCE(config, '{}'), plugin_name, plugin_webhook_id
		FROM channels
		WHERE type = 'webhook' AND direction = 'outbound' AND status = 'enabled'
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhook subscriptions: %w", err)
	}
	defer rows.Close()

	index := &subscriptionIndex{
		byEvent:   make(map[string][]WebhookConfig),
		expiresAt: time.Now().Add(subscriptionCacheTTL),
	}
	for rows.Next() {
		var channelID int
		var channelName string
		var configJSON string
		var pluginName, pluginWebhookID *string

		if err := rows.Scan(&channelID, &channelName, &configJSON, &pluginName, &pluginWebhookID); err != nil {
			logger.Get().Error("Failed to scan webhook channel", "error", err)
			continue
		}

		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			logger.Get().Error("Failed to parse webhook config", "error", err, "channel_id", channelID)
			continue
		}

		if !config.WebhookAutoTrigger {
			continue
		}

		secret, decryptErr := email.DecryptOrLegacy(w.encryption, config.WebhookSecret)
		if decryptErr != nil {
			logger.Get().Error("Failed to decrypt webhook secret", "error", decryptErr, "channel_id", channelID)
			continue
		}

		webhook := WebhookConfig{
			ChannelID:        channelID,
			Name:             channelName,
			URL:              config.WebhookURL,
			Secret:           secret,
			Headers:          config.WebhookHeaders,
			ScopeType:        config.WebhookScopeType,
			WorkspaceIDs:     config.WebhookWorkspaceIDs,
			CollectionIDs:    config.WebhookCollectionIDs,
			AutoTrigger:      config.WebhookAutoTrigger,
			SubscribedEvents: config.WebhookSubscribedEvents,
			PluginHandler:    config.WebhookPluginHandler,
		}

		if pluginName != nil && *pluginName != "" {
			webhook.PluginName = *pluginName
		}
		if pluginWebhookID != nil && *pluginWebhookID != "" {
			webhook.PluginWebhookID = *pluginWebhookID
		}

		for _, subscribedEvent := range webhook.SubscribedEvents {
			// Plugin handlers may subscribe to internal lifecycle events; external
			// webhooks are limited to the events exposed by the editor.
			if webhook.PluginName == "" && !supportedAutomaticEvents[subscribedEvent] {
				continue
			}
			index.byEvent[subscribedEvent] = append(index.byEvent[subscribedEvent], webhook)
		}
		index.entries++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate webhook subscriptions: %w", err)
	}
	w.subscriptions = index
	return index, nil
}

// Collection scope never matches until a QL evaluator is wired, preventing unintended delivery.
// TODO(channel-bughunt #11): wire collection QL evaluation and restore.
func (w *WebhookSender) matchesScope(ctx context.Context, config *models.ChannelConfig, item *models.Item) bool {
	_ = ctx
	switch config.WebhookScopeType {
	case "all", "":
		return true
	case "workspaces":
		return w.contains(config.WebhookWorkspaceIDs, item.WorkspaceID)
	case "collections":
		slog.Warn("webhook collection scope is not yet evaluated; treating as no-match",
			"item_id", item.ID,
			"collection_ids", config.WebhookCollectionIDs,
		)
		return false
	}
	return false
}

// contains checks if a slice contains a value
func (w *WebhookSender) contains(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// ValidateWebhookURL checks that a webhook URL is safe to call (not targeting internal networks).
// Exported so that admin endpoints (e.g. handlers.UpdateChannelConfig) can
// reject SSRF-shaped URLs at write time, rather than only at send time.
func ValidateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("webhook URL must not contain user credentials")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL must have a host")
	}

	// Reject localhost and common loopback names
	lower := net.ParseIP(host)
	if lower == nil {
		// It's a hostname, resolve it
		if host == "localhost" || host == "ip6-localhost" || host == "ip6-loopback" {
			return fmt.Errorf("webhook URL must not target localhost")
		}
		lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ips, err := net.DefaultResolver.LookupHost(lookupCtx, host)
		if err != nil {
			return fmt.Errorf("cannot resolve webhook host %q: %w", host, err)
		}
		for _, ipStr := range ips {
			if ip := net.ParseIP(ipStr); ip != nil && isPrivateIP(ip) {
				return fmt.Errorf("webhook URL %q resolves to private IP %s", host, ipStr)
			}
		}
	} else if isPrivateIP(lower) {
		return fmt.Errorf("webhook URL must not target private IP %s", host)
	}

	return nil
}

// isPrivateIP returns true for loopback, private, and link-local addresses.
func isPrivateIP(ip net.IP) bool {
	return utils.IsBlockedSSRFAddr(ip)
}

func (w *WebhookSender) itemPayloadJSON(ctx context.Context, event string, item *models.Item) (json.RawMessage, error) {
	fullItem, err := w.itemRepository.FindByIDWithDetailsContext(ctx, item.ID)
	if err != nil {
		// Deleted items no longer exist by the time the asynchronous worker runs.
		// The mutation path supplied the best available snapshot, so serialize it
		// instead of dropping every item.deleted delivery.
		if event != "item.deleted" {
			return nil, err
		}
		fullItem = item
	}
	itemResponse := dto.MapItemToResponse(fullItem, "")
	itemJSON, err := json.Marshal(itemResponse)
	if err != nil {
		return nil, fmt.Errorf("serialize webhook item: %w", err)
	}
	return itemJSON, nil
}

func (w *WebhookSender) recordPayloadFailure(ctx context.Context, webhook WebhookConfig, event string, itemID int, err error) {
	delivery := &models.WebhookDelivery{
		ChannelID:    webhook.ChannelID,
		ItemID:       &itemID,
		EventType:    event,
		AttemptType:  attemptTypeFor(event),
		Transport:    "http",
		RequestedAt:  time.Now().UTC(),
		ErrorMessage: "failed to load item: " + err.Error(),
	}
	if webhook.PluginName != "" {
		delivery.Transport = "plugin"
	}
	w.recordDelivery(ctx, delivery)
}

func (w *WebhookSender) destinationLimit(channelID int) chan struct{} {
	w.destinationMu.Lock()
	defer w.destinationMu.Unlock()
	if w.destinationLimits == nil {
		w.destinationLimits = make(map[int]chan struct{})
	}
	limit := w.destinationLimits[channelID]
	if limit == nil {
		limit = make(chan struct{}, destinationConcurrency)
		w.destinationLimits[channelID] = limit
	}
	return limit
}

func (w *WebhookSender) observeDeliveryLatency(elapsed time.Duration) {
	nanos := elapsed.Nanoseconds()
	w.deliveryCount.Add(1)
	w.deliveryNanos.Add(nanos)
	for {
		current := w.maxDeliveryNS.Load()
		if nanos <= current || w.maxDeliveryNS.CompareAndSwap(current, nanos) {
			return
		}
	}
}

// sendWebhookPayload sends one already-hydrated item payload to a configured
// URL or plugin. Hydration happens once per event, not once per destination.
func (w *WebhookSender) sendWebhookPayload(parentCtx context.Context, webhook WebhookConfig, event string, itemID int, itemJSON json.RawMessage) error {
	deliveryStart := time.Now()
	defer func() { w.observeDeliveryLatency(time.Since(deliveryStart)) }()

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	limit := w.destinationLimit(webhook.ChannelID)
	select {
	case limit <- struct{}{}:
		defer func() { <-limit }()
	case <-ctx.Done():
		w.recordPayloadFailure(ctx, webhook, event, itemID, ctx.Err())
		return ctx.Err()
	}

	delivery := &models.WebhookDelivery{
		ChannelID:   webhook.ChannelID,
		ItemID:      &itemID,
		EventType:   event,
		AttemptType: attemptTypeFor(event),
		Transport:   "http",
		RequestedAt: time.Now().UTC(),
	}

	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		WebhookID: webhook.ChannelID,
		Item:      itemJSON,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Get().Error("Failed to serialize webhook payload", "error", err)
		delivery.ErrorMessage = "failed to serialize payload: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return fmt.Errorf("serialize webhook payload: %w", err)
	}

	if webhook.PluginName != "" {
		delivery.Transport = "plugin"
		delivery.RequestURL = "" // not meaningful for plugin transport

		if w.pluginDispatcher == nil {
			logger.Get().Error("Plugin dispatcher not configured, cannot send plugin webhook",
				"plugin", webhook.PluginName,
				"webhook_id", webhook.PluginWebhookID,
			)
			delivery.ErrorMessage = "plugin dispatcher not configured"
			w.recordDelivery(ctx, delivery)
			return fmt.Errorf("plugin dispatcher is not configured")
		}

		pluginStart := time.Now()
		if err = w.pluginDispatcher.DispatchToPlugin(ctx, webhook.PluginName, webhook.PluginHandler, event, payloadBytes); err != nil {
			logger.Get().Error("Failed to dispatch webhook to plugin",
				"error", err,
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
			)
			delivery.ErrorMessage = err.Error()
		} else {
			logger.Get().Debug("Plugin webhook dispatched",
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
				"item_id", itemID,
			)
			delivery.Success = true
		}
		ms := int(time.Since(pluginStart).Milliseconds())
		delivery.ResponseTimeMs = &ms
		w.recordDelivery(ctx, delivery)
		if !delivery.Success {
			return fmt.Errorf("plugin webhook delivery failed: %s", delivery.ErrorMessage)
		}
		return nil
	}

	delivery.RequestURL = redactedWebhookURL(webhook.URL)

	if err := ValidateWebhookURL(webhook.URL); err != nil {
		logger.Get().Error("Webhook URL validation failed", "error", err, "url", delivery.RequestURL, "webhook_id", webhook.ChannelID)
		delivery.ErrorMessage = "URL validation failed: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return fmt.Errorf("webhook URL validation failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		logger.Get().Error("Failed to create webhook request", "error", err, "url", delivery.RequestURL)
		delivery.ErrorMessage = "failed to create request: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return fmt.Errorf("create webhook request: %w", err)
	}

	// Apply custom headers FIRST so reserved Windshift headers (Content-Type,
	// X-Webhook-*) overwrite any collision below. Previously the order was
	// reversed and a channel manager could supply an X-Webhook-Signature
	// custom header that overrode the computed HMAC.
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Reserved Windshift headers take precedence over custom headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-ID", fmt.Sprintf("%d", webhook.ChannelID))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	if webhook.Secret != "" {
		signature := w.generateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	httpStart := time.Now()
	resp, err := w.httpClient.Do(req)
	elapsedMs := int(time.Since(httpStart).Milliseconds())
	delivery.ResponseTimeMs = &elapsedMs

	if err != nil {
		logger.Get().Error("Failed to send webhook", "error", err, "url", delivery.RequestURL, "webhook_id", webhook.ChannelID)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
		delivery.ErrorMessage = err.Error()
		w.recordDelivery(ctx, delivery)
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	delivery.ResponseStatusCode = &resp.StatusCode

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Get().Debug("Webhook sent successfully", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, true)
		delivery.Success = true
	} else {
		logger.Get().Warn("Webhook returned non-success status", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
		delivery.ErrorMessage = fmt.Sprintf("non-2xx status: %d", resp.StatusCode)
		// Capture a bounded slice of the response body so operators can see
		// the receiver's error verbatim (e.g. "invalid signature", "rate
		// limited") instead of just the status code. LimitReader caps memory
		// at 4 KiB even when a misbehaving receiver streams gigabytes.
		preview, perr := io.ReadAll(io.LimitReader(resp.Body, webhookResponsePreviewBytes))
		if perr == nil && len(preview) > 0 {
			delivery.ResponsePreview = string(preview)
		}
	}
	w.recordDelivery(ctx, delivery)
	if !delivery.Success {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func redactedWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// webhookResponsePreviewBytes caps how much of a non-2xx response body we
// store on each delivery row. 4 KiB is enough for typical error messages and
// stays well clear of TEXT/BLOB column overhead on both backends.
const webhookResponsePreviewBytes = 4 * 1024

// ErrWebhookDisabled is returned by TriggerManually when the target webhook
// channel exists but is currently in 'disabled' status. Callers map this to
// a 400 so the operator sees the precise reason instead of "not found".
var ErrWebhookDisabled = fmt.Errorf("webhook is disabled")

// TriggerManually sends a webhook manually for a specific item.
// This is used when webhooks are triggered from item actions, not events.
func (w *WebhookSender) TriggerManually(ctx context.Context, webhookID, itemID int) error {
	// Get webhook config. Status is loaded so disabled webhooks fail loudly
	// instead of silently delivering; GetMatchingWebhooks already filters
	// the automatic path on status, but the manual path bypassed it before.
	var (
		channelName string
		status      string
		configJSON  string
	)
	query := "SELECT name, status, COALESCE(config, '{}') FROM channels WHERE id = ? AND type = 'webhook' AND direction = 'outbound'"
	err := w.db.QueryRowContext(ctx, query, webhookID).Scan(&channelName, &status, &configJSON)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}
	if status != "enabled" {
		return ErrWebhookDisabled
	}

	var config models.ChannelConfig
	if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
		return fmt.Errorf("failed to parse webhook config: %w", err)
	}
	secret, err := email.DecryptOrLegacy(w.encryption, config.WebhookSecret)
	if err != nil {
		return fmt.Errorf("decrypt webhook secret: %w", err)
	}

	// Get item
	dbStart := time.Now()
	item, err := w.itemRepository.FindByIDWithDetailsContext(ctx, itemID)
	w.databaseNanos.Add(time.Since(dbStart).Nanoseconds())
	if err != nil {
		return fmt.Errorf("item not found: %w", err)
	}

	// Check scope matching
	if !w.matchesScope(ctx, &config, item) {
		return fmt.Errorf("item does not match webhook scope")
	}

	// Build webhook config
	webhook := WebhookConfig{
		ChannelID: webhookID,
		Name:      channelName,
		URL:       config.WebhookURL,
		Secret:    secret,
		Headers:   config.WebhookHeaders,
	}

	// Send synchronously for manual triggers
	itemJSON, err := json.Marshal(dto.MapItemToResponse(item, ""))
	if err != nil {
		return fmt.Errorf("serialize webhook item: %w", err)
	}
	if err := w.sendWebhookPayload(ctx, webhook, "manual", item.ID, itemJSON); err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	return nil
}

// SendTestWebhook sends a test webhook to verify configuration
func (w *WebhookSender) SendTestWebhook(ctx context.Context, config *models.ChannelConfig) (success bool, message string) {
	if config.WebhookURL == "" {
		return false, "Webhook URL is required"
	}

	// Validate URL to prevent SSRF
	if err := ValidateWebhookURL(config.WebhookURL); err != nil {
		return false, fmt.Sprintf("Invalid webhook URL: %v", err)
	}

	// Create test payload
	testPayload := map[string]any{
		"event":     "test",
		"timestamp": time.Now().UTC(),
		"message":   "This is a test webhook from Windshift",
		"item": map[string]any{
			"id":    0,
			"title": "Test Item",
			"workspace": map[string]any{
				"id":   0,
				"name": "Test Workspace",
				"key":  "TEST",
			},
		},
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		return false, "Failed to create test payload"
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", config.WebhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return false, fmt.Sprintf("Failed to create request: %v", err)
	}

	// Apply custom headers FIRST so reserved Windshift headers overwrite any
	// collision (especially X-Webhook-Signature). See sendWebhook for rationale.
	for key, value := range config.WebhookHeaders {
		req.Header.Set(key, value)
	}

	// Set reserved headers — these take precedence over custom headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "test")
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add signature if secret is configured
	if config.WebhookSecret != "" {
		signature := w.generateSignature(payloadBytes, config.WebhookSecret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Send request through an SSRF-safe client (validate-then-dial gap closed
	// at the dialer; URL form already validated by ValidateWebhookURL above).
	client := newSSRFSafeWebhookClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Failed to send webhook: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, fmt.Sprintf("Test webhook sent successfully (status: %d)", resp.StatusCode)
	}

	return false, fmt.Sprintf("Webhook returned non-success status: %d", resp.StatusCode)
}

// generateSignature creates HMAC-SHA256 signature for webhook payload
func (w *WebhookSender) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// updateChannelActivity updates the last_activity timestamp for a channel
func (w *WebhookSender) updateChannelActivity(ctx context.Context, channelID int, _ bool) {
	start := time.Now()
	defer func() { w.databaseNanos.Add(time.Since(start).Nanoseconds()) }()
	query := "UPDATE channels SET last_activity = ? WHERE id = ?"
	_, _ = w.db.ExecWriteContext(ctx, query, time.Now(), channelID)
}
