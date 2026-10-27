package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recordingBlobRemover struct {
	mu    sync.Mutex
	paths []string
	ch    chan string
}

func (r *recordingBlobRemover) Remove(_ context.Context, path string) error {
	r.mu.Lock()
	r.paths = append(r.paths, path)
	r.mu.Unlock()
	select {
	case r.ch <- path:
	default:
	}
	return nil
}

func TestEnqueueAttachmentDeletionsRunsAsync(t *testing.T) {
	remover := &recordingBlobRemover{ch: make(chan string, 4)}
	SetItemAttachmentBlobRemover(remover)

	enqueueAttachmentDeletions([]string{"items/1/a.png", ""})

	select {
	case got := <-remover.ch:
		if got != "items/1/a.png" {
			t.Fatalf("removed path = %q, want items/1/a.png", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the background deletion worker")
	}
}
