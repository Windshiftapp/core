package services

import "context"

// BlobRemover is the attachment-storage capability used when an item subtree is
// deleted. *blobstore.Store implements it.
type BlobRemover interface {
	Remove(ctx context.Context, storedPath string) error
}

var itemAttachmentBlobRemover BlobRemover

// SetItemAttachmentBlobRemover wires the attachment backend used by item
// deletion. Item deletes happen through many ItemCRUDService instances, so a
// single startup hook keeps every surface covered. Called once from server
// wiring, before the server serves requests.
func SetItemAttachmentBlobRemover(r BlobRemover) {
	itemAttachmentBlobRemover = r
}
