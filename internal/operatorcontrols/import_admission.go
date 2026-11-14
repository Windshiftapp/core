package operatorcontrols

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var ErrAdmissionUnavailable = errors.New("operator import admission unavailable")

// ImportAdmission is the application boundary for operator-controlled batch
// capacity. Implementations must hold the permit until Release, including on
// cancellation while the worker unwinds.
type ImportAdmission interface {
	Acquire(context.Context) (func(), error)
}

// ImportEstimate carries non-secret planning inputs to admission. Nil totals
// mean readiness has not established an estimate, not zero-cost work.
type ImportEstimate struct {
	JobID                  string
	Projects               int
	AssetSchemas           int
	WorkerCeiling          int
	AttachmentCeilingBytes int64
	BlobAllowanceBytes     int64
	EstimatedIssues        *int64
	EstimatedBlobBytes     *int64
}

type importEstimateKey struct{}

func WithImportEstimate(ctx context.Context, estimate ImportEstimate) context.Context {
	return context.WithValue(ctx, importEstimateKey{}, estimate)
}

func ImportEstimateFromContext(ctx context.Context) (ImportEstimate, bool) {
	estimate, ok := ctx.Value(importEstimateKey{}).(ImportEstimate)
	return estimate, ok
}

// FileImportAdmission coordinates isolated instances sharing one local host
// directory. Locks release on process death. Cross-host fencing is external.
type FileImportAdmission struct {
	Directory string
	Slots     int
}

func (a FileImportAdmission) TryAcquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(a.Directory)
	if err != nil {
		return nil, fmt.Errorf("%w: open admission directory", ErrAdmissionUnavailable)
	}
	defer func() { _ = root.Close() }()
	for slot := 0; slot < a.Slots; slot++ {
		file, err := root.OpenFile(fmt.Sprintf("import-%d.lock", slot), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, fmt.Errorf("%w: open admission slot", ErrAdmissionUnavailable)
		}
		locked, err := tryImportLock(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("%w: lock admission slot", ErrAdmissionUnavailable)
		}
		if locked {
			var once sync.Once
			return func() { once.Do(func() { _ = file.Close() }) }, nil
		}
		_ = file.Close()
	}
	return nil, &LimitError{Resource: "import_slots"}
}

func (a FileImportAdmission) Acquire(ctx context.Context) (func(), error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		release, err := a.TryAcquire(ctx)
		var limited *LimitError
		if !errors.As(err, &limited) {
			return release, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Controls) ImportAdmission() (ImportAdmission, time.Duration) {
	if c == nil || c.policy.Imports == nil {
		return nil, 0
	}
	p := c.policy.Imports
	return FileImportAdmission{Directory: p.AdmissionDirectory, Slots: p.Slots}, time.Duration(p.WaitSeconds) * time.Second
}

type importPermitKey struct{}

func (c *Controls) WithImportPermit(ctx context.Context) context.Context {
	return context.WithValue(ctx, importPermitKey{}, c)
}

// CheckImport keeps other bulk importers unavailable when only Jira admission is qualified.
// Nested work in an admitted Jira import shares its existing permit.
func (c *Controls) CheckImport(ctx context.Context, kind string) error {
	if err := c.Check(Imports); err != nil {
		return err
	}
	if c != nil && c.policy.Imports != nil && kind != "jira" && ctx.Value(importPermitKey{}) != c {
		return &DeniedError{Capability: Imports}
	}
	return nil
}
