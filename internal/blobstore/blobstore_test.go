package blobstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStoreRoundTrip(t *testing.T) {
	store := NewLocal(t.TempDir())
	if !store.Enabled() {
		t.Fatal("store should be enabled")
	}
	if store.Backend() != "local" {
		t.Fatalf("backend = %q, want local", store.Backend())
	}

	ctx := context.Background()
	storedPath := store.Path("items", "7", "note.txt")
	if !filepath.IsAbs(storedPath) {
		t.Fatalf("local Path() = %q, want absolute", storedPath)
	}

	data := []byte("hello attachments")
	if err := store.Put(ctx, storedPath, bytes.NewReader(data), int64(len(data)), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	obj, err := store.Open(ctx, storedPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(obj)
	_ = obj.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("round-trip data = %q, want %q", got, data)
	}
	if obj.Size() != int64(len(data)) {
		t.Fatalf("Size() = %d, want %d", obj.Size(), len(data))
	}

	// Relative stored paths (the shape email ingestion writes) resolve against
	// the same root.
	relPath := filepath.Join("items", "7", "relative.txt")
	if err := store.Put(ctx, relPath, bytes.NewReader(data), int64(len(data)), "text/plain"); err != nil {
		t.Fatalf("Put relative: %v", err)
	}
	relObj, err := store.Open(ctx, relPath)
	if err != nil {
		t.Fatalf("Open relative: %v", err)
	}
	_ = relObj.Close()

	if err := store.Remove(ctx, storedPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := store.Open(ctx, storedPath); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after Remove = %v, want ErrNotFound", err)
	}
}

func TestLocalStoreRejectsTraversal(t *testing.T) {
	store := NewLocal(t.TempDir())
	ctx := context.Background()
	err := store.Put(ctx, "../../etc/passwd", bytes.NewReader([]byte("x")), 1, "text/plain")
	if !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("Put traversal = %v, want ErrOutsideRoot", err)
	}
}

func TestS3KeyNormalisation(t *testing.T) {
	b := &s3Backend{prefix: "attachments", localRoot: "/data/attachments"}

	tests := []struct {
		name    string
		stored  string
		want    string
		wantErr bool
	}{
		{name: "absolute under root", stored: "/data/attachments/items/1/a.png", want: "attachments/items/1/a.png"},
		{name: "relative key", stored: "items/1/a.png", want: "attachments/items/1/a.png"},
		{name: "nested relative", stored: "pages/2/diagram.json", want: "attachments/pages/2/diagram.json"},
		{name: "traversal", stored: "../../etc/passwd", wantErr: true},
		{name: "absolute outside root", stored: "/etc/passwd", wantErr: true},
		{name: "empty", stored: "  ", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := b.key(tc.stored)
			if tc.wantErr {
				if !errors.Is(err, ErrOutsideRoot) {
					t.Fatalf("key(%q) error = %v, want ErrOutsideRoot", tc.stored, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("key(%q): %v", tc.stored, err)
			}
			if got != tc.want {
				t.Fatalf("key(%q) = %q, want %q", tc.stored, got, tc.want)
			}
		})
	}
}

func TestDisabledStore(t *testing.T) {
	var store *Store
	if store.Enabled() {
		t.Fatal("nil store should be disabled")
	}
	if err := store.Put(context.Background(), "x", bytes.NewReader(nil), 0, ""); err == nil {
		t.Fatal("Put on disabled store should fail")
	}
	if _, err := store.Open(context.Background(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open on disabled store = %v, want ErrNotFound", err)
	}
}

func TestLocalStoreHealth(t *testing.T) {
	root := t.TempDir()
	store := NewLocal(root)
	if err := store.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	// The readiness probe must not create files.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("health check created files: %v", entries)
	}

	missing := NewLocal(filepath.Join(root, "missing"))
	if err := missing.HealthCheck(context.Background()); err == nil {
		t.Fatal("HealthCheck on a missing root should fail")
	}

	if err := Disabled().HealthCheck(context.Background()); err != nil {
		t.Fatalf("disabled HealthCheck = %v, want nil", err)
	}
}
