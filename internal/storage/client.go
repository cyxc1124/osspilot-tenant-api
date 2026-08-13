package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var (
	ErrNotFound = errors.New("not found")
	ErrNotEmpty = errors.New("not empty")
)

type Config struct {
	Endpoint    string
	AccessKey   string
	SecretKey   string
	Region      string
	UploadTTL   time.Duration
	DownloadTTL time.Duration
}

func (c Config) Ready() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != ""
}

type Client struct {
	s3          *s3.Client
	presign     *s3.PresignClient
	uploadTTL   time.Duration
	downloadTTL time.Duration
}

type ObjectMeta struct {
	Size        int64
	ETag        *string
	ContentType *string
}

type CompletedPart struct {
	PartNumber int32
	ETag       string
}

func New(cfg Config) *Client {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	awscfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}
	cli := s3.NewFromConfig(awscfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(strings.TrimRight(cfg.Endpoint, "/"))
		o.UsePathStyle = true // ponytail: RGW expects path-style; virtual-host if a vendor requires it
	})
	up, down := cfg.UploadTTL, cfg.DownloadTTL
	if up <= 0 {
		up = 30 * time.Minute
	}
	if down <= 0 {
		down = 10 * time.Minute
	}
	return &Client{s3: cli, presign: s3.NewPresignClient(cli), uploadTTL: up, downloadTTL: down}
}

func (c *Client) EnsureBucket(ctx context.Context, name string) error {
	_, err := c.s3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
	if err == nil {
		return nil
	}
	var exists *types.BucketAlreadyExists
	var owned *types.BucketAlreadyOwnedByYou
	if errors.As(err, &exists) || errors.As(err, &owned) {
		return nil
	}
	return err
}

func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	_, err := c.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return ErrNotFound
	}
	if apiCode(err) == "BucketNotEmpty" {
		return ErrNotEmpty
	}
	return err
}

func (c *Client) HeadObject(ctx context.Context, bucket, key string) (ObjectMeta, error) {
	out, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if isNotFound(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, err
	}
	meta := ObjectMeta{ContentType: out.ContentType, ETag: stripETag(out.ETag)}
	if out.ContentLength != nil {
		meta.Size = *out.ContentLength
	}
	return meta, nil
}

func (c *Client) PutObject(ctx context.Context, bucket, key string, body io.Reader, contentType string) error {
	in := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: body}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := c.s3.PutObject(ctx, in)
	return err
}

func (c *Client) PresignPut(ctx context.Context, bucket, key, contentType string) (string, int, error) {
	in := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := c.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(c.uploadTTL))
	if err != nil {
		return "", 0, err
	}
	return out.URL, int(c.uploadTTL.Seconds()), nil
}

func (c *Client) PresignGet(ctx context.Context, bucket, key string) (string, int, error) {
	out, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(c.downloadTTL))
	if err != nil {
		return "", 0, err
	}
	return out.URL, int(c.downloadTTL.Seconds()), nil
}

func (c *Client) PresignUploadPart(ctx context.Context, bucket, key, uploadID string, part int32) (string, error) {
	out, err := c.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
		UploadId: aws.String(uploadID), PartNumber: aws.Int32(part),
	}, s3.WithPresignExpires(c.uploadTTL))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *Client) CreateMultipart(ctx context.Context, bucket, key, contentType string) (string, error) {
	in := &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := c.s3.CreateMultipartUpload(ctx, in)
	if err != nil {
		return "", err
	}
	if out.UploadId == nil || *out.UploadId == "" {
		return "", errors.New("CreateMultipartUpload did not return UploadId")
	}
	return *out.UploadId, nil
}

func (c *Client) CompleteMultipart(ctx context.Context, bucket, key, uploadID string, parts []CompletedPart) error {
	completed := make([]types.CompletedPart, len(parts))
	for i, p := range parts {
		etag := p.ETag
		n := p.PartNumber
		completed[i] = types.CompletedPart{ETag: &etag, PartNumber: &n}
	}
	_, err := c.s3.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	})
	return err
}

func (c *Client) AbortMultipart(ctx context.Context, bucket, key, uploadID string) error {
	_, err := c.s3.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(uploadID),
	})
	return err
}

func (c *Client) UploadTTLSeconds() int { return int(c.uploadTTL.Seconds()) }

func apiCode(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	return ""
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	var nsb *types.NoSuchBucket
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nsb) || errors.As(err, &nf) {
		return true
	}
	code := apiCode(err)
	return code == "NotFound" || code == "NoSuchKey" || code == "NoSuchBucket" || code == "404"
}

func stripETag(etag *string) *string {
	if etag == nil {
		return nil
	}
	s := strings.Trim(*etag, `"`)
	return &s
}
