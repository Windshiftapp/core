package blobstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3Config configures the S3-compatible backend. AWS is the default target:
// leaving Endpoint empty uses the regional AWS endpoint, and ForcePathStyle
// stays off for virtual-hosted addressing. Self-hosted stores (MinIO, RustFS,
// Wasabi, …) set Endpoint and usually ForcePathStyle.
type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Prefix          string
	ForcePathStyle  bool
	// LocalRoot is the historical ATTACHMENT_PATH. When set, absolute paths
	// written by the local backend are mapped back to object keys, so switching
	// a deployment to S3 does not require rewriting every row.
	LocalRoot string
}

type s3Backend struct {
	client    *s3.Client
	bucket    string
	prefix    string
	localRoot string
}

// NewS3 builds an S3-backed store. It validates configuration but does not make
// a network call; call Probe at startup to check reachability.
//
// Credentials come from cfg.AccessKeyID/SecretAccessKey when both are set
// (S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY); when both are empty the AWS SDK's
// default credential chain resolves them (env vars, shared config/SSO, web
// identity, instance profile). Setting exactly one is a configuration error.
func NewS3(cfg S3Config) (*Store, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, errors.New("S3 bucket is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOpts := make([]func(*awsconfig.LoadOptions) error, 0, 2)
	loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return nil, errors.New("S3 access key id and secret access key must be set together")
		}
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	// Build the client once for the lifetime of the process. The SDK owns
	// connection pooling and retries transient network failures, so there is no
	// manual reconnect path: the same client is reused by every request.
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	describe := "s3://" + bucket
	if prefix != "" {
		describe += "/" + prefix
	}

	return &Store{
		backend: &s3Backend{
			client:    client,
			bucket:    bucket,
			prefix:    prefix,
			localRoot: strings.TrimSpace(cfg.LocalRoot),
		},
		enabled:  true,
		describe: describe,
	}, nil
}

func (b *s3Backend) key(storedPath string) (string, error) {
	p := filepath.ToSlash(strings.TrimSpace(storedPath))
	if p == "" {
		return "", ErrOutsideRoot
	}
	if b.localRoot != "" {
		root := filepath.ToSlash(filepath.Clean(b.localRoot))
		if p == root {
			return "", ErrOutsideRoot
		}
		if strings.HasPrefix(p, root+"/") {
			p = strings.TrimPrefix(p, root+"/")
		} else if path.IsAbs(p) {
			return "", ErrOutsideRoot
		}
	} else if path.IsAbs(p) {
		// Without a configured local root there is no safe prefix to strip.
		return "", ErrOutsideRoot
	}
	p = strings.TrimLeft(p, "/")
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", ErrOutsideRoot
	}
	if b.prefix != "" {
		return b.prefix + "/" + clean, nil
	}
	return clean, nil
}

func (b *s3Backend) put(ctx context.Context, storedPath string, r io.Reader, size int64, contentType string) error {
	key, err := b.key(storedPath)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if _, err := b.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put s3 object %q: %w", key, err)
	}
	return nil
}

func (b *s3Backend) open(ctx context.Context, storedPath string) (Object, error) {
	key, err := b.key(storedPath)
	if err != nil {
		return nil, err
	}
	head, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("head s3 object %q: %w", key, err)
	}
	return &s3Object{
		ctx:     ctx,
		client:  b.client,
		bucket:  b.bucket,
		key:     key,
		size:    aws.ToInt64(head.ContentLength),
		modTime: aws.ToTime(head.LastModified),
	}, nil
}

func (b *s3Backend) remove(ctx context.Context, storedPath string) error {
	key, err := b.key(storedPath)
	if err != nil {
		return err
	}
	// DeleteObject is idempotent: deleting a missing key still succeeds.
	if _, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete s3 object %q: %w", key, err)
	}
	return nil
}

func (b *s3Backend) probe(ctx context.Context) error {
	sentinelKey, keyErr := b.key(".windshift-probe")
	return probeWithRetry(ctx, defaultProbePolicy, func(ctx context.Context) error {
		_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(b.bucket)})
		if err == nil {
			return nil
		}
		if ClassifyProbeError(err) != ProbeAccessDenied {
			return err
		}

		// Some S3-compatible servers (SeaweedFS among them) answer HeadBucket
		// and HeadObject with a bare 403 for both a missing s3:ListBucket grant
		// and invalid credentials. Confirm write access with a zero-byte
		// sentinel: a credential-shaped error on Put is fatal, while a
		// successful write means only ListBucket is missing.
		if keyErr != nil {
			return err
		}
		_, headErr := b.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(sentinelKey),
		})
		if headErr != nil && !isS3NotFound(headErr) {
			if cls := ClassifyProbeError(headErr); cls == ProbeCredential || cls == ProbeTransient {
				return headErr
			}
		}

		_, putErr := b.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(b.bucket),
			Key:           aws.String(sentinelKey),
			Body:          bytes.NewReader(nil),
			ContentLength: aws.Int64(0),
		})
		if putErr != nil {
			if cls := ClassifyProbeError(putErr); cls == ProbeCredential || cls == ProbeTransient {
				return putErr
			}
			return err
		}
		_, _ = b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(sentinelKey),
		})
		// Credentials and writes work; only ListBucket is missing.
		return err
	})
}

// health is the readiness variant: one HeadBucket, no retry loop and no
// sentinel write. AccessDenied means the bucket is reachable (only
// s3:ListBucket is missing), so it reports healthy; transport and credential
// failures do not.
func (b *s3Backend) health(ctx context.Context) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(b.bucket)})
	if err == nil {
		return nil
	}
	if ClassifyProbeError(err) == ProbeAccessDenied {
		return nil
	}
	return err
}

// s3Object lazily fetches byte ranges, so http.ServeContent can seek without
// buffering the whole object in memory.
type s3Object struct {
	ctx     context.Context
	client  *s3.Client
	bucket  string
	key     string
	size    int64
	modTime time.Time
	pos     int64
	body    io.ReadCloser
}

func (o *s3Object) Read(p []byte) (int, error) {
	if o.pos >= o.size {
		return 0, io.EOF
	}
	if o.body == nil {
		resp, err := o.client.GetObject(o.ctx, &s3.GetObjectInput{
			Bucket: aws.String(o.bucket),
			Key:    aws.String(o.key),
			Range:  aws.String(fmt.Sprintf("bytes=%d-", o.pos)),
		})
		if err != nil {
			return 0, err
		}
		o.body = resp.Body
	}
	n, err := o.body.Read(p)
	o.pos += int64(n)
	if err == io.EOF {
		_ = o.body.Close()
		o.body = nil
		if o.pos >= o.size {
			return n, io.EOF
		}
		return n, nil
	}
	return n, err
}

func (o *s3Object) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = o.pos + offset
	case io.SeekEnd:
		next = o.size + offset
	default:
		return 0, errors.New("invalid seek whence")
	}
	if next < 0 {
		return 0, errors.New("cannot seek before start of object")
	}
	if next != o.pos {
		if o.body != nil {
			_ = o.body.Close()
			o.body = nil
		}
		o.pos = next
	}
	return o.pos, nil
}

func (o *s3Object) Close() error {
	if o.body == nil {
		return nil
	}
	err := o.body.Close()
	o.body = nil
	return err
}

func (o *s3Object) Size() int64        { return o.size }
func (o *s3Object) ModTime() time.Time { return o.modTime }

func isS3NotFound(err error) bool {
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket", "404":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	return false
}
