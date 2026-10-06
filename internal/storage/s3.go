package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/AXmishell/axmipic/internal/config"
)

// S3 将对象存储在任何兼容 S3 的服务中（AWS S3、MinIO、Cloudflare
// R2、阿里云 OSS、腾讯云 COS 等）。
type S3 struct {
	client        *minio.Client
	bucket        string
	publicBaseURL string
	pathStyle     bool
	endpointHost  string
	secure        bool
}

// NewS3 创建一个兼容 S3 的存储驱动。
func NewS3(cfg config.S3Config) (*S3, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("storage: s3.bucket must not be empty")
	}
	host, secure, err := parseEndpoint(cfg.Endpoint, cfg.Secure)
	if err != nil {
		return nil, err
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: secure,
		Region: cfg.Region,
	}
	if cfg.UsePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}
	client, err := minio.New(host, opts)
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{
		client:        client,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		pathStyle:     cfg.UsePathStyle,
		endpointHost:  host,
		secure:        secure,
	}, nil
}

// Put 上传一个对象。
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if _, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("storage: s3 put %q: %w", key, err)
	}
	return nil
}

// Get 打开一个对象以供读取。
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 get %q: %w", key, mapS3Error(err))
	}
	// GetObject 是惰性的；Stat 会强制发起请求，因此对象缺失会在此时报告，
	// 而不是在首次读取时。
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("storage: s3 get %q: %w", key, mapS3Error(err))
	}
	return obj, nil
}

// Delete 删除一个对象。删除不存在的对象是无操作。
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: s3 delete %q: %w", key, err)
	}
	return nil
}

// Exists 报告对象是否存在。
func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("storage: s3 stat %q: %w", key, err)
	}
	return true, nil
}

// Stat 返回已存储对象的元数据。
func (s *S3) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 stat %q: %w", key, mapS3Error(err))
	}
	return &ObjectInfo{Key: key, Size: info.Size, ContentType: info.ContentType}, nil
}

// URL 返回对象键对应的公开 URL。
func (s *S3) URL(key string) string {
	if s.publicBaseURL != "" {
		return s.publicBaseURL + "/" + key
	}
	scheme := "https"
	if !s.secure {
		scheme = "http"
	}
	if s.pathStyle {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpointHost, s.bucket, key)
	}
	return fmt.Sprintf("%s://%s.%s/%s", scheme, s.bucket, s.endpointHost, key)
}

// List 枚举桶中的全部对象，供孤儿对象对账使用。
func (s *S3) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("storage: s3 list: %w", obj.Err)
		}
		objects = append(objects, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified,
		})
	}
	return objects, nil
}

// PresignPut 签发一个 S3 POST 策略，客户端用它来直接上传。
// 该策略固定确切的键、媒体类型和大小范围。
func (s *S3) PresignPut(ctx context.Context, key string, opts PresignOptions) (*PresignedRequest, error) {
	expiresAt := time.Now().Add(opts.Expires)
	policy := minio.NewPostPolicy()
	if err := policy.SetBucket(s.bucket); err != nil {
		return nil, fmt.Errorf("storage: s3 policy bucket: %w", err)
	}
	if err := policy.SetKey(key); err != nil {
		return nil, fmt.Errorf("storage: s3 policy key: %w", err)
	}
	if err := policy.SetExpires(expiresAt); err != nil {
		return nil, fmt.Errorf("storage: s3 policy expires: %w", err)
	}
	if opts.ContentType != "" {
		if err := policy.SetContentType(opts.ContentType); err != nil {
			return nil, fmt.Errorf("storage: s3 policy content-type: %w", err)
		}
	}
	if opts.MaxSize > 0 {
		if err := policy.SetContentLengthRange(1, opts.MaxSize); err != nil {
			return nil, fmt.Errorf("storage: s3 policy size range: %w", err)
		}
	}
	u, fields, err := s.client.PresignedPostPolicy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("storage: s3 presign: %w", err)
	}
	if opts.ContentType != "" {
		fields["Content-Type"] = opts.ContentType
	}
	return &PresignedRequest{
		URL:       u.String(),
		Method:    "POST",
		Fields:    fields,
		ExpiresAt: expiresAt,
	}, nil
}

func parseEndpoint(endpoint string, secureDefault bool) (string, bool, error) {
	if strings.TrimSpace(endpoint) == "" {
		return "", false, fmt.Errorf("storage: s3.endpoint must not be empty")
	}
	if strings.HasPrefix(endpoint, "https://") {
		return strings.TrimPrefix(endpoint, "https://"), true, nil
	}
	if strings.HasPrefix(endpoint, "http://") {
		return strings.TrimPrefix(endpoint, "http://"), false, nil
	}
	return endpoint, secureDefault, nil
}

func isS3NotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket" ||
		resp.Code == "NotFound" || resp.StatusCode == 404
}

// mapS3Error 将「未找到」类 S3 错误转换为 ErrNotFound，同时保留
// 服务层所使用的存储错误契约。
func mapS3Error(err error) error {
	if isS3NotFound(err) {
		return fmt.Errorf("storage: s3: %w", ErrNotFound)
	}
	return err
}
