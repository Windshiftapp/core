package services

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// BlobRemover is the attachment-storage capability used when an item subtree is
// deleted. *blobstore.Store implements it.
type BlobRemover interface {
	Remove(ctx context.Context, storedPath string) error
}

const (
	// attachmentDeletionQueueSize bounds the in-flight cleanup backlog. A full
	// queue drops the path (the reviewer accepted an orphaned object as the
	// trade-off for keeping deletes off the database's critical path); a
	// durable outbox is the follow-up if that becomes a problem.
	attachmentDeletionQueueSize = 1024
	attachmentDeletionWorkers   = 4
	attachmentDeletionTimeout   = time.Minute
)

var (
	itemAttachmentBlobRemover BlobRemover
	attachmentDeletionQueue   chan string
	attachmentDeletionOnce    sync.Once
)

// SetItemAttachmentBlobRemover wires the attachment backend used by item
// deletion and starts the background blob-removal workers. Item deletes happen
// through many ItemCRUDService instances, so a single startup hook keeps every
// surface covered. Called once from server wiring, before the server serves
// requests.
func SetItemAttachmentBlobRemover(r BlobRemover) {
	itemAttachmentBlobRemover = r
	attachmentDeletionOnce.Do(func() {
		attachmentDeletionQueue = make(chan string, attachmentDeletionQueueSize)
		for i := 0; i < attachmentDeletionWorkers; i++ {
			go drainAttachmentDeletionQueue()
		}
	})
}

// enqueueAttachmentDeletions hands committed blob paths to the background
// workers. It is deliberately non-blocking: a slow object store must never hold
// up an item delete or a database transaction.
func enqueueAttachmentDeletions(paths []string) {
	if attachmentDeletionQueue == nil {
		return
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		select {
		case attachmentDeletionQueue <- path:
		default:
			slog.Warn("attachment deletion queue is full; leaving the blob for later cleanup",
				slog.String("component", "attachments"), slog.String("path", path))
		}
	}
}

func drainAttachmentDeletionQueue() {
	for path := range attachmentDeletionQueue {
		remover := itemAttachmentBlobRemover
		if remover == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), attachmentDeletionTimeout)
		err := remover.Remove(ctx, path)
		cancel()
		if err != nil {
			slog.Warn("failed to delete item attachment blob",
				slog.String("component", "attachments"), slog.String("path", path), slog.Any("error", err))
		}
	}
}
