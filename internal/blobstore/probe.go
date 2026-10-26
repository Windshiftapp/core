package blobstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// ProbeClass categorizes a startup probe failure so the retry policy can decide
// whether to retry, continue, or fail fast.
type ProbeClass int

const (
	// ProbeOK means the probe succeeded.
	ProbeOK ProbeClass = iota
	// ProbeTransient covers connection, DNS, TLS, timeout, throttling and 5xx
	// failures: worth retrying with backoff.
	ProbeTransient
	// ProbeAccessDenied means the bucket is reachable but the caller lacks
	// s3:ListBucket. Get/Put may still work, so startup continues with a warning.
	ProbeAccessDenied
	// ProbeCredential means the credentials are invalid or expired: a config
	// error that retrying cannot fix.
	ProbeCredential
	// ProbeFatal covers deterministic errors such as NoSuchBucket.
	ProbeFatal
)

type probePolicy struct {
	attempts int
	timeout  time.Duration
	delays   []time.Duration
}

var defaultProbePolicy = probePolicy{
	attempts: 5,
	timeout:  3 * time.Second,
	delays:   []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second},
}

// ClassifyProbeError maps an S3 error to a retry decision. It is a pure
// function so the taxonomy can be tested without a live bucket.
func ClassifyProbeError(err error) ProbeClass {
	if err == nil {
		return ProbeOK
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDenied", "Forbidden":
			return ProbeAccessDenied
		case "InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken",
			"InvalidToken", "TokenRefreshRequired", "InvalidClientTokenId",
			"UnrecognizedClientException":
			return ProbeCredential
		case "NoSuchBucket", "NotFound", "404":
			return ProbeFatal
		case "SlowDown", "RequestTimeout", "InternalError", "ServiceUnavailable",
			"Throttling", "RequestLimitExceeded", "500", "502", "503", "504":
			return ProbeTransient
		}
	}

	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		switch status := respErr.HTTPStatusCode(); {
		case status >= 500:
			return ProbeTransient
		case status == http.StatusForbidden:
			return ProbeAccessDenied
		case status >= 400:
			return ProbeFatal
		}
	}

	// Transport failures (refused connections, DNS, TLS handshakes, timeouts)
	// are transient by default.
	return ProbeTransient
}

// probeWithRetry runs fn until it succeeds, a non-retryable error is returned,
// or the attempt budget is exhausted.
func probeWithRetry(ctx context.Context, p probePolicy, fn func(context.Context) error) error {
	if p.attempts < 1 {
		p.attempts = 1
	}
	if p.timeout <= 0 {
		p.timeout = 3 * time.Second
	}

	var last error
	for attempt := 1; attempt <= p.attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, p.timeout)
		err := fn(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		last = err

		switch ClassifyProbeError(err) {
		case ProbeAccessDenied:
			slog.Warn("attachment object storage is reachable but access is denied; continuing (grant s3:ListBucket to silence this)",
				slog.String("component", "blobstore"), slog.Any("error", err))
			return nil
		case ProbeCredential, ProbeFatal:
			return err
		}

		if attempt == p.attempts {
			break
		}
		delay := p.delays[len(p.delays)-1]
		if attempt-1 < len(p.delays) {
			delay = p.delays[attempt-1]
		}
		select {
		case <-time.After(jitter(delay)):
		case <-ctx.Done():
			return fmt.Errorf("%w (last probe error: %v)", ctx.Err(), last)
		}
	}
	return fmt.Errorf("attachment object storage probe failed after %d attempts: %w", p.attempts, last)
}

// jitter returns a delay in the upper half of d, so retries spread out without
// exceeding the documented budget.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	half := d / 2
	// Retry jitter only needs to spread attempts; unpredictability is not a
	// security property here.
	return half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // retry jitter, not a security-sensitive value
}
