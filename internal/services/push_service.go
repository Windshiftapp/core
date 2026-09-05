package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/config"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// system_settings keys under which an auto-provisioned VAPID keypair is
// persisted. The private key lives at the same trust boundary as the push
// subscription keys already in the database and is never returned by any
// settings API.
const (
	settingVAPIDPublicKey  = "push_vapid_public_key"
	settingVAPIDPrivateKey = "push_vapid_private_key"
)

// ResolveVAPIDConfig guarantees a usable VAPID keypair so Web Push works with
// zero operator configuration. Resolution precedence:
//
//  1. Explicit env (both VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY set) — returned
//     untouched so operators can manage and rotate keys out-of-band.
//  2. A keypair previously generated and persisted in system_settings.
//  3. A freshly generated keypair, persisted for every future boot.
//
// On any error it returns cfg unchanged (push stays disabled) rather than
// failing startup — push is a non-critical feature.
func ResolveVAPIDConfig(db database.Database, cfg config.PushConfig, log *slog.Logger) config.PushConfig {
	// (1) Explicit env override wins and is never persisted.
	if cfg.VAPIDPublicKey != "" && cfg.VAPIDPrivateKey != "" {
		return cfg
	}

	repo := repository.NewSystemSettingRepository(db)
	pub, pubOK, errPub := repo.GetValue(settingVAPIDPublicKey)
	priv, privOK, errPriv := repo.GetValue(settingVAPIDPrivateKey)
	if err := errors.Join(errPub, errPriv); err != nil {
		log.Error("reading persisted VAPID keys; Web Push disabled", "error", err)
		return cfg
	}

	// (2) Reuse a previously persisted pair.
	if pubOK && privOK && pub != "" && priv != "" {
		cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey = pub, priv
		return cfg
	}

	// (3) First run with no keys anywhere: generate and persist a pair.
	// GenerateVAPIDKeys returns (privateKey, publicKey, err) — order matters.
	newPriv, newPub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Error("generating VAPID keypair; Web Push disabled", "error", err)
		return cfg
	}
	if err := repo.Upsert(settingVAPIDPublicKey, newPub, "string",
		"Auto-generated VAPID public key for Web Push", "push"); err != nil {
		log.Error("persisting VAPID public key; Web Push disabled this boot", "error", err)
		return cfg
	}
	if err := repo.Upsert(settingVAPIDPrivateKey, newPriv, "string",
		"Auto-generated VAPID private key for Web Push", "push"); err != nil {
		log.Error("persisting VAPID private key; Web Push disabled this boot", "error", err)
		return cfg
	}
	cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey = newPub, newPriv
	log.Info("auto-generated and persisted a VAPID keypair for Web Push")
	return cfg
}

// pushTTLSeconds is how long a push service should retain an undelivered
// message. One day matches the notification cache horizon.
const pushTTLSeconds = 86400

// maxPushBodyLen caps the notification snippet placed in the push payload. Push
// payloads intentionally carry only IDs + a short title/body + a target URL; the
// app fetches full content after opening (see plan security note).
const maxPushBodyLen = 140

// activeSubsWhere filters to a user's non-revoked subscriptions. Shared by
// List and deliver so the "active" invariant (revoked_at IS NULL) is defined
// once rather than duplicated per query.
const activeSubsWhere = "WHERE user_id = ? AND revoked_at IS NULL"

// ErrInvalidEndpoint rejects a subscription endpoint that fails SSRF
// validation (non-HTTPS, credentials, or a non-public destination).
var ErrInvalidEndpoint = errors.New("invalid push endpoint")

// endpointValidationTimeout bounds the DNS lookup performed while validating
// an endpoint hostname so a slow resolver can't stall the subscribe handler.
const endpointValidationTimeout = 3 * time.Second

// lookupEndpointIPs resolves a hostname for endpoint validation. It is a
// variable so tests can stub DNS without touching the network.
var lookupEndpointIPs = func(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

// blockedEndpointCIDRs covers non-public ranges that net.IP's helpers don't
// classify on their own: CGNAT shared space, IETF protocol assignments,
// benchmarking, and the reserved class-E block.
var blockedEndpointCIDRs = func() []*net.IPNet {
	cidrs := []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4"}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}()

// isPublicEndpointIP reports whether ip is a globally routable unicast
// address a push service could legitimately live on.
func isPublicEndpointIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedEndpointCIDRs {
		if blocked.Contains(ip) {
			return false
		}
	}
	return true
}

// validatePushEndpoint enforces the SSRF policy for subscription endpoints:
// absolute HTTPS URL, no embedded credentials, valid port, and a destination
// (IP literal or every resolved address) that is publicly routable.
func validatePushEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("%w: must be an absolute https URL without credentials", ErrInvalidEndpoint)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidEndpoint)
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%w: invalid port", ErrInvalidEndpoint)
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicEndpointIP(ip) {
			return fmt.Errorf("%w: %s is not a public address", ErrInvalidEndpoint, host)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), endpointValidationTimeout)
	defer cancel()
	ips, err := lookupEndpointIPs(ctx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("%w: cannot resolve %s", ErrInvalidEndpoint, host)
	}
	for _, ip := range ips {
		if !isPublicEndpointIP(ip) {
			return fmt.Errorf("%w: %s resolves to a non-public address", ErrInvalidEndpoint, host)
		}
	}
	return nil
}

// PushSubscriptionInfo is the non-sensitive view of a stored subscription
// returned to the owning user (keys are never exposed back to the client).
type PushSubscriptionInfo struct {
	ID         int        `json:"id"`
	Endpoint   string     `json:"endpoint"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// pushPayload is the compact JSON delivered to the service worker's push handler.
type pushPayload struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

// PushServiceConfig bounds queued notifications, concurrent remote work, and
// total delivery time. Push is best-effort after the durable notification row
// commits, so overflow is explicit and metriced rather than blocking inserts.
type PushServiceConfig struct {
	QueueSize           int
	Workers             int
	DeliveryTimeout     time.Duration
	HTTPTimeout         time.Duration
	MaxSubscriptions    int
	MaxRetries          int
	RetryInitialBackoff time.Duration
	ShutdownTimeout     time.Duration
}

func DefaultPushServiceConfig() PushServiceConfig {
	return PushServiceConfig{
		QueueSize:           1000,
		Workers:             4,
		DeliveryTimeout:     15 * time.Second,
		HTTPTimeout:         5 * time.Second,
		MaxSubscriptions:    20,
		MaxRetries:          2,
		RetryInitialBackoff: 100 * time.Millisecond,
		ShutdownTimeout:     20 * time.Second,
	}
}

type pushJob struct {
	notification models.Notification
	enqueuedAt   time.Time
}

type pushSubscription struct {
	id       int
	endpoint string
	auth     string
	p256dh   string
}

type webPushSender func(context.Context, []byte, *webpush.Subscription, *webpush.Options) (*http.Response, error)

// PushService stores per-user Web Push subscriptions and dispatches compact
// push messages when notifications are created. It is a no-op when VAPID keys
// are not configured (Enabled() == false).
type PushService struct {
	db               database.Database
	cfg              config.PushConfig
	serviceCfg       PushServiceConfig
	httpClient       *http.Client
	send             webPushSender
	permService      *PermissionService
	notificationAuth *NotificationAuthorizer

	queue        chan pushJob
	enqueueMu    sync.RWMutex
	closed       bool
	closeOnce    sync.Once
	workerCtx    context.Context
	workerCancel context.CancelFunc
	wg           sync.WaitGroup
	userLocks    sync.Map // map[int]*sync.Mutex

	enqueued             int64
	jobsDropped          int64
	jobsProcessed        int64
	deliveryErrors       int64
	retries              int64
	subscriptionsSent    int64
	subscriptionsDropped int64
	activeWorkers        int64
	maxActiveWorkers     int64
	lastQueueWaitNS      int64
	maxQueueWaitNS       int64
	lastDeliveryNS       int64
	maxDeliveryNS        int64
}

// NewPushService constructs a PushService. The returned service is safe to use
// even when push is disabled — every method degrades to a no-op / empty result.
func NewPushService(db database.Database, cfg config.PushConfig, permissionServices ...*PermissionService) *PushService {
	return newPushService(db, cfg, DefaultPushServiceConfig(), webpush.SendNotificationWithContext, permissionServices...)
}

func newPushService(db database.Database, cfg config.PushConfig, serviceCfg PushServiceConfig, sender webPushSender, permissionServices ...*PermissionService) *PushService {
	defaults := DefaultPushServiceConfig()
	if serviceCfg.QueueSize <= 0 {
		serviceCfg.QueueSize = defaults.QueueSize
	}
	if serviceCfg.Workers <= 0 {
		serviceCfg.Workers = defaults.Workers
	}
	if serviceCfg.DeliveryTimeout <= 0 {
		serviceCfg.DeliveryTimeout = defaults.DeliveryTimeout
	}
	if serviceCfg.HTTPTimeout <= 0 {
		serviceCfg.HTTPTimeout = defaults.HTTPTimeout
	}
	if serviceCfg.MaxSubscriptions <= 0 {
		serviceCfg.MaxSubscriptions = defaults.MaxSubscriptions
	}
	if serviceCfg.MaxRetries < 0 {
		serviceCfg.MaxRetries = 0
	}
	if serviceCfg.RetryInitialBackoff <= 0 {
		serviceCfg.RetryInitialBackoff = defaults.RetryInitialBackoff
	}
	if serviceCfg.ShutdownTimeout <= 0 {
		serviceCfg.ShutdownTimeout = defaults.ShutdownTimeout
	}
	workerCtx, workerCancel := context.WithCancel(context.Background()) //nolint:gosec // retained and called by CloseContext
	service := &PushService{
		db:         db,
		cfg:        cfg,
		serviceCfg: serviceCfg,
		httpClient: &http.Client{
			Timeout:   serviceCfg.HTTPTimeout,
			Transport: utils.ConfigureHTTPTransport(nil),
			// Redirects re-run the endpoint SSRF policy so a validated
			// endpoint cannot bounce delivery to an internal address.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return validatePushEndpoint(req.URL.String())
			},
		},
		send:         sender,
		queue:        make(chan pushJob, serviceCfg.QueueSize),
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}
	if len(permissionServices) > 0 {
		service.permService = permissionServices[0]
	}
	service.notificationAuth = NewNotificationAuthorizer(db, service.permService, nil)
	if service.Enabled() {
		service.wg.Add(serviceCfg.Workers)
		for workerID := 0; workerID < serviceCfg.Workers; workerID++ {
			go service.worker(workerID)
		}
	}
	return service
}

// SetNotificationAuthorizer wires current workspace and asset authorization
// checks into queued push delivery.
func (s *PushService) SetNotificationAuthorizer(authorizer *NotificationAuthorizer) {
	if authorizer != nil {
		s.notificationAuth = authorizer
	}
}

// Enabled reports whether Web Push is configured.
func (s *PushService) Enabled() bool { return s.cfg.Enabled() }

// PublicKey returns the VAPID public key the browser needs to subscribe.
func (s *PushService) PublicKey() string { return s.cfg.VAPIDPublicKey }

// Subscribe upserts a subscription for the user, keyed by endpoint. Re-subscribing
// an existing endpoint (e.g. after key rotation) refreshes its keys, reattaches
// it to the current user, and clears any revoked marker.
func (s *PushService) Subscribe(userID int, endpoint, authKey, p256dhKey, userAgent string) error {
	if err := validatePushEndpoint(endpoint); err != nil {
		return err
	}
	_, err := s.db.ExecWrite(`
		INSERT INTO push_subscriptions (user_id, endpoint, auth_key, p256dh_key, user_agent, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = excluded.user_id,
			auth_key = excluded.auth_key,
			p256dh_key = excluded.p256dh_key,
			user_agent = excluded.user_agent,
			revoked_at = NULL,
			last_used_at = CURRENT_TIMESTAMP
	`, userID, endpoint, authKey, p256dhKey, userAgent)
	return err
}

// List returns the user's active (non-revoked) subscriptions, newest first.
func (s *PushService) List(userID int) ([]PushSubscriptionInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, endpoint, user_agent, created_at, last_used_at FROM push_subscriptions "+
			activeSubsWhere+" ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PushSubscriptionInfo{}
	for rows.Next() {
		var info PushSubscriptionInfo
		var lastUsed sql.NullTime
		if err := rows.Scan(&info.ID, &info.Endpoint, &info.UserAgent, &info.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			info.LastUsedAt = &t
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// Delete removes one of the user's subscriptions by id. The user_id predicate
// enforces ownership — a user can never delete another user's subscription.
func (s *PushService) Delete(userID, id int) error {
	_, err := s.db.ExecWrite(`DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// mobileItemRoutePattern matches the item deep link emitted by
// notification_service.itemActionURL (/workspaces/<id>/items/<itemId>) so we
// can rewrite it to the mobile item route the PWA service worker navigates to.
// It also defensively matches a nested-collection shape
// (.../collections/<cid>/items/<itemId>) should an action URL ever carry one,
// since the trailing /items/<id> segment is all the rewrite needs.
var mobileItemRoutePattern = regexp.MustCompile(`/items/(\d+)(?:[/?#]|$)`)

// mobileActionURL rewrites a notification's desktop action URL into its mobile
// equivalent for the push payload. Item deep links (/workspaces/.../items/N)
// become /m/items/N; any non-item URL (or empty value) is returned as-is so
// callers can fall back to the generic mobile notifications route.
func mobileActionURL(actionURL string) string {
	if actionURL == "" {
		return ""
	}
	if m := mobileItemRoutePattern.FindStringSubmatch(actionURL); m != nil {
		return "/m/items/" + m[1]
	}
	return actionURL
}

// Enqueue adds a durable notification to the bounded best-effort push queue.
// It never blocks notification persistence and reports overflow to the caller.
func (s *PushService) Enqueue(notification models.Notification) bool {
	if !s.Enabled() {
		return true
	}
	s.enqueueMu.RLock()
	defer s.enqueueMu.RUnlock()
	if s.closed {
		atomic.AddInt64(&s.jobsDropped, 1)
		return false
	}
	select {
	case s.queue <- pushJob{notification: notification, enqueuedAt: time.Now()}:
		atomic.AddInt64(&s.enqueued, 1)
		return true
	default:
		atomic.AddInt64(&s.jobsDropped, 1)
		return false
	}
}

func (s *PushService) worker(workerID int) {
	defer s.wg.Done()
	for job := range s.queue {
		wait := time.Since(job.enqueuedAt)
		atomic.StoreInt64(&s.lastQueueWaitNS, int64(wait))
		updatePushMaximum(&s.maxQueueWaitNS, int64(wait))
		active := atomic.AddInt64(&s.activeWorkers, 1)
		updatePushMaximum(&s.maxActiveWorkers, active)
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(s.workerCtx, s.serviceCfg.DeliveryTimeout)
		err := s.dispatch(ctx, job.notification)
		cancel()
		duration := time.Since(startedAt)
		atomic.StoreInt64(&s.lastDeliveryNS, int64(duration))
		updatePushMaximum(&s.maxDeliveryNS, int64(duration))
		atomic.AddInt64(&s.activeWorkers, -1)
		atomic.AddInt64(&s.jobsProcessed, 1)
		if err != nil {
			atomic.AddInt64(&s.deliveryErrors, 1)
			slog.Warn("push dispatch failed", "component", "push", "worker_id", workerID, "notification_id", job.notification.ID, "error", err)
		}
	}
}

func (s *PushService) dispatch(ctx context.Context, notification models.Notification) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("push dispatch panicked", slog.String("component", "push"), slog.Any("recover", recovered))
			err = fmt.Errorf("push dispatch panic: %v", recovered)
		}
	}()
	visible, err := s.notificationVisible(notification)
	if err != nil {
		return err
	}
	if !visible {
		return nil
	}

	body := notification.Message
	if body == "" {
		body = notification.Title
	}
	if len(body) > maxPushBodyLen {
		body = body[:maxPushBodyLen]
	}
	// Push subscriptions are registered exclusively by the mobile PWA, so the
	// deep link we hand the service worker must target the mobile surface. The
	// notification's ActionURL is a desktop route (itemActionURL() emits
	// /workspaces/<id>/items/<itemId>) — rewrite it to /m/items/<itemId> so
	// tapping a push notification doesn't drop the user onto the desktop item
	// page, which is borderline unusable on a phone.
	actionURL := mobileActionURL(notification.ActionURL)
	if actionURL == "" {
		actionURL = "/m/notifications"
	}

	lock, _ := s.userLocks.LoadOrStore(notification.UserID, &sync.Mutex{})
	userLock, ok := lock.(*sync.Mutex)
	if !ok {
		panic("push user lock has unexpected type")
	}
	userLock.Lock()
	defer userLock.Unlock()
	return s.deliver(ctx, notification.UserID, pushPayload{
		ID:    notification.ID,
		Title: notification.Title,
		Body:  body,
		Type:  notification.Type,
		URL:   actionURL,
	})
}

func (s *PushService) notificationVisible(notification models.Notification) (bool, error) {
	snapshot, err := s.notificationAuth.Snapshot(notification.UserID)
	if err != nil {
		return false, err
	}
	return snapshot.Visible(notification)
}

// deliver loads the user's active subscriptions and sends one push each,
// pruning subscriptions the push service reports as permanently gone.
func (s *PushService) deliver(ctx context.Context, userID int, payload pushPayload) error {
	if !s.Enabled() {
		return nil
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, endpoint, auth_key, p256dh_key, COUNT(*) OVER() FROM push_subscriptions "+activeSubsWhere+" ORDER BY id LIMIT ?", userID, s.serviceCfg.MaxSubscriptions)
	if err != nil {
		return fmt.Errorf("load push subscriptions: %w", err)
	}
	var subs []pushSubscription
	totalSubscriptions := 0
	for rows.Next() {
		var sb pushSubscription
		if err := rows.Scan(&sb.id, &sb.endpoint, &sb.auth, &sb.p256dh, &totalSubscriptions); err != nil {
			rows.Close()
			return fmt.Errorf("scan push subscription: %w", err)
		}
		subs = append(subs, sb)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate push subscriptions: %w", err)
	}
	rows.Close()

	if len(subs) == 0 {
		return nil
	}
	if totalSubscriptions > len(subs) {
		atomic.AddInt64(&s.subscriptionsDropped, int64(totalSubscriptions-len(subs)))
	}

	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	options := &webpush.Options{
		HTTPClient:      s.httpClient,
		Subscriber:      s.cfg.VAPIDSubject,
		VAPIDPublicKey:  s.cfg.VAPIDPublicKey,
		VAPIDPrivateKey: s.cfg.VAPIDPrivateKey,
		TTL:             pushTTLSeconds,
	}

	var deliveryErr error
	for _, sb := range subs {
		if err := s.sendSubscription(ctx, message, sb, options); err != nil {
			slog.Warn("push subscription delivery failed", "component", "push", "sub_id", sb.id, "error", err)
			deliveryErr = errors.Join(deliveryErr, err)
		}
	}
	return deliveryErr
}

func (s *PushService) sendSubscription(ctx context.Context, message []byte, sb pushSubscription, options *webpush.Options) error {
	var lastErr error
	for attempt := 0; attempt <= s.serviceCfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := s.send(ctx, message, &webpush.Subscription{
			Endpoint: sb.endpoint,
			Keys:     webpush.Keys{Auth: sb.auth, P256dh: sb.p256dh},
		}, options)
		if err != nil {
			lastErr = err
			if attempt < s.serviceCfg.MaxRetries {
				atomic.AddInt64(&s.retries, 1)
				if err := waitForPushRetry(ctx, s.serviceCfg.RetryInitialBackoff, attempt); err != nil {
					return err
				}
				continue
			}
			return err
		}
		status := resp.StatusCode
		detail := readPushResponse(resp)
		switch {
		case status == http.StatusNotFound || status == http.StatusGone:
			_, err := s.db.ExecWriteContext(ctx, `UPDATE push_subscriptions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?`, sb.id)
			return err
		case status >= 200 && status < 300:
			atomic.AddInt64(&s.subscriptionsSent, 1)
			_, _ = s.db.ExecWriteContext(ctx, `UPDATE push_subscriptions SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, sb.id)
			return nil
		case status == http.StatusTooManyRequests || status >= 500:
			lastErr = fmt.Errorf("push endpoint returned %d: %s", status, detail)
			if attempt < s.serviceCfg.MaxRetries {
				atomic.AddInt64(&s.retries, 1)
				if err := waitForPushRetry(ctx, s.serviceCfg.RetryInitialBackoff, attempt); err != nil {
					return err
				}
				continue
			}
			return lastErr
		default:
			return fmt.Errorf("push endpoint returned %d: %s", status, detail)
		}
	}
	return lastErr
}

func readPushResponse(resp *http.Response) string {
	if resp == nil {
		return "empty response"
	}
	if resp.Body == nil {
		return strings.TrimSpace(resp.Status)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = resp.Status
	}
	return detail
}

func waitForPushRetry(ctx context.Context, initial time.Duration, attempt int) error {
	delay := initial
	for i := 0; i < attempt; i++ {
		if delay > time.Second/2 {
			delay = time.Second
			break
		}
		delay *= 2
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close drains queued jobs up to the configured shutdown deadline.
func (s *PushService) Close(ctx context.Context) error {
	if !s.Enabled() {
		s.workerCancel()
		return nil
	}
	s.closeOnce.Do(func() {
		s.enqueueMu.Lock()
		s.closed = true
		close(s.queue)
		s.enqueueMu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.workerCancel()
		return nil
	case <-ctx.Done():
		s.workerCancel()
		return fmt.Errorf("push shutdown with %d queued and %d active: %w", len(s.queue), atomic.LoadInt64(&s.activeWorkers), ctx.Err())
	}
}

// GetStats returns queue, worker, retry, fan-out, and latency metrics.
func (s *PushService) GetStats() map[string]int64 {
	return map[string]int64{
		"queue_depth":           int64(len(s.queue)),
		"enqueued":              atomic.LoadInt64(&s.enqueued),
		"jobs_dropped":          atomic.LoadInt64(&s.jobsDropped),
		"jobs_processed":        atomic.LoadInt64(&s.jobsProcessed),
		"delivery_errors":       atomic.LoadInt64(&s.deliveryErrors),
		"retries":               atomic.LoadInt64(&s.retries),
		"subscriptions_sent":    atomic.LoadInt64(&s.subscriptionsSent),
		"subscriptions_dropped": atomic.LoadInt64(&s.subscriptionsDropped),
		"active_workers":        atomic.LoadInt64(&s.activeWorkers),
		"max_active_workers":    atomic.LoadInt64(&s.maxActiveWorkers),
		"last_queue_wait_ms":    time.Duration(atomic.LoadInt64(&s.lastQueueWaitNS)).Milliseconds(),
		"max_queue_wait_ms":     time.Duration(atomic.LoadInt64(&s.maxQueueWaitNS)).Milliseconds(),
		"last_delivery_ms":      time.Duration(atomic.LoadInt64(&s.lastDeliveryNS)).Milliseconds(),
		"max_delivery_ms":       time.Duration(atomic.LoadInt64(&s.maxDeliveryNS)).Milliseconds(),
	}
}

func updatePushMaximum(target *int64, value int64) {
	for {
		previous := atomic.LoadInt64(target)
		if value <= previous || atomic.CompareAndSwapInt64(target, previous, value) {
			return
		}
	}
}
