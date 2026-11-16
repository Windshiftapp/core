package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"windshift/internal/operatorcontrols"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Usage counts current physical objects, including staged and orphaned blobs.
// One instance must exclusively own its root/prefix and be its only writer.
type Usage struct {
	Bytes        int64 `json:"bytes"`
	LimitBytes   int64 `json:"limit_bytes"`
	MaxFileBytes int64 `json:"max_file_bytes"`
}

func (s *Store) Usage(ctx context.Context) (Usage, error) {
	if !s.Enabled() {
		return Usage{}, nil
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if s.controls.Limits().BlobBytes == 0 {
		s.quotaKnown = false
	}
	if err := s.loadQuota(ctx); err != nil {
		return Usage{}, err
	}
	return Usage{Bytes: s.quotaBytes, LimitBytes: s.controls.Limits().BlobBytes, MaxFileBytes: s.controls.Limits().AttachmentBytes}, nil
}

func (s *Store) loadQuota(ctx context.Context) error {
	if s.quotaKnown {
		return nil
	}
	inventory, ok := s.backend.(interface {
		usage(context.Context) (int64, error)
	})
	if !ok {
		return errors.New("storage backend cannot account operator quota")
	}
	n, err := inventory.usage(ctx)
	if err != nil {
		return fmt.Errorf("reconcile blob usage: %w", err)
	}
	s.quotaBytes = n
	s.quotaKnown = true
	return nil
}

func (s *Store) objectSize(ctx context.Context, path string) (int64, error) {
	object, err := s.backend.open(ctx, path)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer object.Close()
	return object.Size(), nil
}

func (s *Store) putLimited(ctx context.Context, path string, r io.Reader, size int64, contentType string) error {
	limits := s.controls.Limits()
	if size < 0 || (limits.AttachmentBytes > 0 && size > limits.AttachmentBytes) {
		return &operatorcontrols.LimitError{Resource: "attachment_bytes"}
	}
	reader, err := sizedReader(r, size)
	if err != nil {
		return err
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	var oldSize int64
	if err := ctx.Err(); err != nil {
		return err
	}
	if limits.BlobBytes > 0 {
		if err := s.loadQuota(ctx); err != nil {
			return err
		}
		oldSize, err = s.objectSize(ctx, path)
		if err != nil {
			return err
		}
		if size > oldSize && (s.quotaBytes > limits.BlobBytes || size-oldSize > limits.BlobBytes-s.quotaBytes) {
			return &operatorcontrols.LimitError{Resource: "blob_bytes"}
		}
	}
	bounded := io.Reader(&contextReader{ctx: ctx, reader: reader})
	if seeker, ok := reader.(io.Seeker); ok {
		bounded = &contextReadSeeker{contextReader: contextReader{ctx: ctx, reader: reader}, Seeker: seeker}
	}
	if err := s.backend.put(ctx, path, bounded, size, contentType); err != nil {
		s.quotaKnown = false
		return err
	}
	if limits.BlobBytes > 0 {
		s.quotaBytes += size - oldSize
	} else {
		s.quotaKnown = false
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}
type contextReadSeeker struct {
	contextReader
	io.Seeker
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (s *Store) removeLimited(ctx context.Context, path string) error {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if err := s.loadQuota(ctx); err != nil {
		return err
	}
	size, err := s.objectSize(ctx, path)
	if err != nil {
		return err
	}
	if err := s.backend.remove(ctx, path); err != nil {
		s.quotaKnown = false
		return err
	}
	s.quotaBytes -= size
	if s.quotaBytes < 0 {
		s.quotaKnown = false
	}
	return nil
}

func sizedReader(reader io.Reader, size int64) (io.Reader, error) {
	if seeker, ok := reader.(io.Seeker); ok {
		start, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		end, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		if _, err := seeker.Seek(start, io.SeekStart); err != nil {
			return nil, err
		}
		if end-start != size {
			return nil, errors.New("blob size does not match content")
		}
		return reader, nil
	}
	return &exactReader{reader: reader, remaining: size}, nil
}

type exactReader struct {
	reader    io.Reader
	remaining int64
}

func (r *exactReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, errors.New("blob exceeds declared size")
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	if errors.Is(err, io.EOF) && r.remaining > 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (b *localBackend) usage(ctx context.Context) (int64, error) {
	if _, err := os.Stat(b.root); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(b.root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrOutsideRoot
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("non-regular object in blob root")
		}
		if info.Size() > math.MaxInt64-total {
			return errors.New("blob usage overflow")
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func (b *s3Backend) usage(ctx context.Context) (int64, error) {
	prefix := b.prefix
	if prefix != "" {
		prefix += "/"
	}
	pages := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{Bucket: aws.String(b.bucket), Prefix: aws.String(prefix)})
	var total int64
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, object := range page.Contents {
			size := aws.ToInt64(object.Size)
			if size < 0 || size > math.MaxInt64-total {
				return 0, errors.New("invalid blob inventory size")
			}
			total += size
		}
	}
	return total, nil
}
