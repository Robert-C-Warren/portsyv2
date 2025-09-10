package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// R2Config controls connection and transfer behavior.
type R2Config struct {
	AccountID string // CF account ID (for endpoint)
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string // R2 uses "auto"
	KeyPrefix string // optional prefix with bucket

	// Transfer tunables (sane defaults if zero)
	UploadPartSize      int64 // bytes, e.g. 8<<20
	UploadConcurrency   int   // e.g. 4-8
	DownloadPartSize    int64 // bytes
	DownloadConcurrency int   // e.g. 4-8

	// Presign TTL default (used by Presign* helpers)
	DefaultPresignTTL time.Duration
}

type R2Client struct {
	cfg     R2Config
	client  *s3.Client
	upldr   *manager.Uploader
	dl      *manager.Downloader
	presign *s3.PresignClient
}

func (c *R2Client) BucketName() string {
	return c.cfg.Bucket
}

func (r *R2Client) BuildKey(projectName, hash string) string {
	base := path.Join(projectName, "blobs", hash)
	if r.cfg.KeyPrefix != "" {
		return path.Join(r.cfg.KeyPrefix, base)
	}
	return base
}

func NewR2(ctx context.Context, cfg R2Config) (*R2Client, error) {
	if cfg.Region == "" {
		cfg.Region = "auto"
	}
	if cfg.Bucket == "" || cfg.AccountID == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("missing required R2 config fields")
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws cfg: %w", err)
	}

	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint) // R2 endpoint
		o.UsePathStyle = true                 // R2 requires path-style
		// Keep default retryer; R2 behaves like S3 for idempotent ops.
	})

	upPart := cfg.UploadPartSize
	if upPart <= 0 {
		upPart = 8 << 20 // 8 MiB (R2 minimum is 5 MiB; 8 is a good balance)
	}
	upConc := cfg.UploadConcurrency
	if upConc <= 0 {
		upConc = 4
	}
	downPart := cfg.DownloadPartSize
	if downPart <= 0 {
		downPart = 8 << 20
	}
	downConc := cfg.DownloadConcurrency
	if downConc <= 0 {
		downConc = 4
	}

	upldr := manager.NewUploader(s3c, func(u *manager.Uploader) {
		u.PartSize = upPart
		u.Concurrency = upConc
	})
	dl := manager.NewDownloader(s3c, func(d *manager.Downloader) {
		d.PartSize = downPart
		d.Concurrency = downConc
	})

	presigner := s3.NewPresignClient(s3c)

	if cfg.DefaultPresignTTL <= 0 {
		cfg.DefaultPresignTTL = 15 * time.Minute
	}

	return &R2Client{
		cfg:     cfg,
		client:  s3c,
		upldr:   upldr,
		dl:      dl,
		presign: presigner,
	}, nil
}

// ---- Upload options (content-type, metadata) ----
type UploadOpt func(*s3.PutObjectInput)

func WithContentType(ct string) UploadOpt {
	return func(in *s3.PutObjectInput) { in.ContentType = aws.String(ct) }
}

func WithMetadata(kv map[string]string) UploadOpt {
	return func(in *s3.PutObjectInput) {
		if len(kv) == 0 {
			return
		}
		if in.Metadata == nil {
			in.Metadata = map[string]string{}
		}
		for k, v := range kv {
			in.Metadata[k] = v
		}
	}
}

// UploadFile uploads the file at localPath to key. Returns key on success.
func (r *R2Client) UploadFile(ctx context.Context, localPath, key string, opts ...UploadOpt) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open upload file: %w", err)
	}
	defer f.Close()
	return r.uploadReader(ctx, f, key, opts...)
}

func (r *R2Client) DownloadTo(ctx context.Context, key, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("ensure parent dir: %w", err)
	}

	tmp := dstPath + ".part"
	tf, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	// Ensure cleanup on failure
	defer func() {
		_ = tf.Close()
		_ = os.Remove(tmp)
	}()

	_, err = r.dl.Download(ctx, tf, &s3.GetObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if notFound(err) {
			return fmt.Errorf("r2 key not found: %s", key)
		}
		return fmt.Errorf("download key=%s: %w", key, err)
	}
	// Flush file to disk before rename (important on Windows)
	if err := tf.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tf.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	// Best-effort: fsync parent dir to persist rename
	if dir, err := os.Open(filepath.Dir(dstPath)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (r *R2Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if notFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("head key=%s: %w", key, err)
	}
	return true, nil
}

func (r *R2Client) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete key=%s: %w", key, err)
	}
	return nil
}

// BuildR2Key is a legacy helper retained for compatibility.
// Prefer R2Client.BuildKey which respects KeyPrefix.
func BuildR2Key(projectName, relPath, hash string) string {
	return path.Join(projectName, "blobs", hash)
}

// UploadReader uploads a stream to key.
func (r *R2Client) UploadReader(ctx context.Context, rd io.Reader, key string, opts ...UploadOpt) error {
	_, err := r.uploadReader(ctx, rd, key, opts...)
	return err
}

func (r *R2Client) uploadReader(ctx context.Context, rd io.Reader, key string, opts ...UploadOpt) (string, error) {
	in := &s3.PutObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
		Body:   rd,
	}
	for _, o := range opts {
		o(in)
	}
	_, err := r.upldr.Upload(ctx, in)
	if err != nil {
		return "", fmt.Errorf("upload to r2 key=%s: %w", key, err)
	}
	return key, nil
}

// --- Presign helpers ---
func (r *R2Client) PresignGet(ctx context.Context, key string, ttl ...time.Duration) (string, error) {
	expires := r.cfg.DefaultPresignTTL
	if len(ttl) > 0 && ttl[0] > 0 {
		expires = ttl[0]
	}
	out, err := r.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presign get key=%s: %w", key, err)
	}
	return out.URL, nil
}

func (r *R2Client) PresignPut(ctx context.Context, key string, ttl ...time.Duration) (string, http.Header, error) {
	expires := r.cfg.DefaultPresignTTL
	if len(ttl) > 0 && ttl[0] > 0 {
		expires = ttl[0]
	}
	out, err := r.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", nil, fmt.Errorf("presign put key=%s: %w", key, err)
	}
	return out.URL, out.SignedHeader, nil
}

// --- internal helpers ---

func notFound(err error) bool {
	// Smithy API error with HTTP status
	var api smithy.APIError
	if errors.As(err, &api) {
		if respErr, ok := err.(*smithyhttp.ResponseError); ok {
			return respErr.Response.StatusCode == http.StatusNotFound
		}
		// Some errors expose Code/Status via APIError
		if api.ErrorCode() == "NoSuchKey" {
			return true
		}
	}
	// Fallback: check ResponseError directly
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.Response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

func (c *R2Client) UploadFileIfNoneMatch(ctx context.Context, localPath, key, ifNoneMatch string) (*s3.PutObjectOutput, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	in := &s3.PutObjectInput{
		Bucket:      aws.String(c.BucketName()), // <- use exported field
		Key:         aws.String(key),
		Body:        f,
		IfNoneMatch: aws.String(ifNoneMatch), // usually "*"
	}
	out, err := c.client.PutObject(ctx, in)
	if isPreconditionFailed(err) {
		// someone else already put it; that's success for idempotent push
		return nil, nil
	}
	return out, err
}

func isPreconditionFailed(err error) bool {
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == 412 {
		return true
	}
	return false
}

// CopyObject issues a server-side copy (cheap layout migration).
func (c *R2Client) CopyObject(ctx context.Context, fromKey, toKey string) error {
	if fromKey == toKey {
		return nil
	}
	copySource := url.PathEscape(c.BucketName() + "/" + fromKey)
	_, err := c.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(c.BucketName()),
		Key:        aws.String(toKey),
		CopySource: aws.String(copySource),
	})
	return err
}

// UploadIfMissing remains the convenience wrapper your sync.go expects.
func (c *R2Client) UploadIfMissing(ctx context.Context, local, key string) error {
	exists, err := c.Exists(ctx, key)
	if err == nil && exists {
		return nil
	}
	_, err = c.UploadFileIfNoneMatch(ctx, local, key, "*")
	if isPreconditionFailed(err) {
		return nil
	}
	return err
}

func (c *R2Client) CopyIfMissing(ctx context.Context, fromKey, toKey string) error {
	if fromKey == toKey {
		return nil
	}
	exists, err := c.Exists(ctx, toKey)
	if err == nil && exists {
		return nil
	}
	return c.CopyObject(ctx, fromKey, toKey)
}
