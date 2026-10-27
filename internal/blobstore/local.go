package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"windshift/internal/fileserve"
)

type localBackend struct {
	root string
}

func (b *localBackend) put(ctx context.Context, storedPath string, r io.Reader, _ int64, _ string) error {
	abs, err := fileserve.ResolveUnderRoot(b.root, storedPath)
	if err != nil {
		if errors.Is(err, fileserve.ErrOutsideRoot) {
			return ErrOutsideRoot
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // trusted root, numeric entity ids
		return fmt.Errorf("create attachment directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".windshift-*.part")
	if err != nil {
		return fmt.Errorf("create attachment temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write attachment: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod attachment: %w", err)
	}
	// Flush before the rename so a crash cannot leave a committed row pointing
	// at a zero-length or partial object.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync attachment: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close attachment: %w", err)
	}
	if err := os.Rename(tmpName, abs); err != nil {
		cleanup()
		return fmt.Errorf("commit attachment: %w", err)
	}
	return nil
}

func (b *localBackend) open(_ context.Context, storedPath string) (Object, error) {
	file, err := fileserve.OpenUnderRoot(b.root, storedPath)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, ErrNotFound
		case errors.Is(err, fileserve.ErrOutsideRoot):
			return nil, ErrOutsideRoot
		default:
			return nil, err
		}
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &localObject{File: file, size: info.Size(), modTime: info.ModTime()}, nil
}

func (b *localBackend) remove(_ context.Context, storedPath string) error {
	if err := fileserve.RemoveUnderRoot(b.root, storedPath); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil
		case errors.Is(err, fileserve.ErrOutsideRoot):
			return ErrOutsideRoot
		default:
			return err
		}
	}
	return nil
}

func (b *localBackend) probe(_ context.Context) error {
	info, err := os.Stat(b.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", b.root)
	}
	testFile, err := os.CreateTemp(b.root, ".windshift-write-test-*")
	if err != nil {
		return err
	}
	name := testFile.Name()
	if err := testFile.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

// health is the read-only readiness variant: it verifies the root is present
// and is a directory without creating a probe file.
func (b *localBackend) health(_ context.Context) error {
	info, err := os.Stat(b.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", b.root)
	}
	return nil
}

type localObject struct {
	*os.File
	size    int64
	modTime time.Time
}

func (o *localObject) Size() int64        { return o.size }
func (o *localObject) ModTime() time.Time { return o.modTime }
