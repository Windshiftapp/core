package blobstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

func TestClassifyProbeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ProbeClass
	}{
		{name: "nil", err: nil, want: ProbeOK},
		{name: "access denied", err: &smithy.GenericAPIError{Code: "AccessDenied"}, want: ProbeAccessDenied},
		{name: "forbidden", err: &smithy.GenericAPIError{Code: "Forbidden"}, want: ProbeAccessDenied},
		{name: "bad key", err: &smithy.GenericAPIError{Code: "InvalidAccessKeyId"}, want: ProbeCredential},
		{name: "bad signature", err: &smithy.GenericAPIError{Code: "SignatureDoesNotMatch"}, want: ProbeCredential},
		{name: "missing bucket", err: &smithy.GenericAPIError{Code: "NoSuchBucket"}, want: ProbeFatal},
		{name: "throttled", err: &smithy.GenericAPIError{Code: "SlowDown"}, want: ProbeTransient},
		{name: "server error", err: &smithy.GenericAPIError{Code: "InternalError"}, want: ProbeTransient},
		{name: "transport", err: errors.New("connection refused"), want: ProbeTransient},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyProbeError(tc.err); got != tc.want {
				t.Fatalf("ClassifyProbeError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestProbeWithRetryTransientThenSuccess(t *testing.T) {
	attempts := 0
	policy := probePolicy{attempts: 3, timeout: time.Second, delays: []time.Duration{0, 0}}
	err := probeWithRetry(context.Background(), policy, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return &smithy.GenericAPIError{Code: "SlowDown"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("probeWithRetry: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestProbeWithRetryStopsOnFatal(t *testing.T) {
	attempts := 0
	policy := probePolicy{attempts: 5, timeout: time.Second, delays: []time.Duration{0}}
	err := probeWithRetry(context.Background(), policy, func(context.Context) error {
		attempts++
		return &smithy.GenericAPIError{Code: "NoSuchBucket"}
	})
	if err == nil {
		t.Fatal("expected fatal probe error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on fatal)", attempts)
	}
}

func TestProbeWithRetryToleratesAccessDenied(t *testing.T) {
	attempts := 0
	policy := probePolicy{attempts: 5, timeout: time.Second, delays: []time.Duration{0}}
	err := probeWithRetry(context.Background(), policy, func(context.Context) error {
		attempts++
		return &smithy.GenericAPIError{Code: "AccessDenied"}
	})
	if err != nil {
		t.Fatalf("AccessDenied should continue startup, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestProbeWithRetryExhaustsBudget(t *testing.T) {
	attempts := 0
	policy := probePolicy{attempts: 3, timeout: time.Second, delays: []time.Duration{0, 0}}
	err := probeWithRetry(context.Background(), policy, func(context.Context) error {
		attempts++
		return &smithy.GenericAPIError{Code: "ServiceUnavailable"}
	})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
