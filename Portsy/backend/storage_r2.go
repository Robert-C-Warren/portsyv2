package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type R2Config struct {
	AccountID string // CF account ID (for endpoint)
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string // e.g. "auto"
	KeyPrefix string
}

type R2Client struct {
	cfg    R2Config
	client *s3.Client
	upldr  *manager.Uploader
	dl     *manager.Downloader
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
		cfg.Region = "auto" // R2 requires "auto"
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, err
	}

	// ✅ Use service-level options instead of global endpoint resolver
	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint) // <— replacement for the deprecated resolver
		o.UsePathStyle = true                 // R2 needs path-style
	})

	return &R2Client{
		cfg:    cfg,
		client: s3c,
		upldr:  manager.NewUploader(s3c, func(u *manager.Uploader) { u.PartSize = 8 * 1024 * 1024 }),
		dl:     manager.NewDownloader(s3c),
	}, nil
}

func (r *R2Client) UploadFile(ctx context.Context, localPath, key string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = r.upldr.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	return key, err
}

func (r *R2Client) DownloadTo(ctx context.Context, key, dstPath string) error {
	tmp := dstPath + ".part"
	tf, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer tf.Close()

	_, err = r.dl.Download(ctx, tf, &s3.GetObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = tf.Close()
	return os.Rename(tmp, dstPath)
}

func (r *R2Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var re *smithyhttp.ResponseError
		if errors.As(err, &re) && re.Response.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *R2Client) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	return err
}

// Helper to compute a remote key convention, e.g. by content hash.
func BuildR2Key(projectName, relPath, hash string) string {
	// content-addressable storage keeps dedup simple:
	// projects/<project>/blobs/<hash>   and we remember original path in metadata
	return path.Join(projectName, "blobs", hash)
}

// In case you need streaming upload (e.g., from memory)
func (r *R2Client) UploadReader(ctx context.Context, rd io.Reader, key string) error {
	_, err := r.upldr.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
		Body:   rd,
	})
	return err
}
