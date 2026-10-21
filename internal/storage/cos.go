package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"

	"github.com/AXmishell/axmipic/internal/config"
)

// cosDefaultPresignExpiry 是未配置 presign_expiry_sec 时的签名有效期。
const cosDefaultPresignExpiry = 15 * time.Minute

// COS 将对象存储在腾讯云对象存储 COS 中。
type COS struct {
	client        *cos.Client
	bucketURL     *url.URL
	bucketName    string
	domain        string
	secretID      string
	secretKey     string
	private       bool
	presignExpiry time.Duration
}

// NewCOS 创建一个腾讯云 COS 存储驱动。
//
// Bucket 需填写完整的「名称-APPID」形式，例如 axmipic-1250000000；
// Region 例如 ap-guangzhou。
func NewCOS(cfg config.COSConfig) (*COS, error) {
	secretID := strings.TrimSpace(cfg.SecretID)
	secretKey := strings.TrimSpace(cfg.SecretKey)
	bucketName := strings.TrimSpace(cfg.Bucket)
	region := strings.TrimSpace(cfg.Region)
	if secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("storage: cos.secret_id and cos.secret_key must not be empty")
	}
	if bucketName == "" {
		return nil, fmt.Errorf("storage: cos.bucket must not be empty")
	}
	if region == "" {
		return nil, fmt.Errorf("storage: cos.region must not be empty")
	}

	scheme := "https"
	if !cfg.UseHTTPS {
		scheme = "http"
	}
	bucketURL, err := url.Parse(fmt.Sprintf("%s://%s.cos.%s.myqcloud.com", scheme, bucketName, region))
	if err != nil {
		return nil, fmt.Errorf("storage: cos endpoint: %w", err)
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	expiry := time.Duration(cfg.PresignExpirySec) * time.Second
	if expiry <= 0 {
		expiry = cosDefaultPresignExpiry
	}

	return &COS{
		client:        client,
		bucketURL:     bucketURL,
		bucketName:    bucketName,
		domain:        strings.TrimRight(strings.TrimSpace(cfg.Domain), "/"),
		secretID:      secretID,
		secretKey:     secretKey,
		private:       cfg.Private,
		presignExpiry: expiry,
	}, nil
}

// Put 上传一个对象。
func (c *COS) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: contentType},
	}
	if _, err := c.client.Object.Put(ctx, key, r, opt); err != nil {
		return fmt.Errorf("storage: cos put %q: %w", key, err)
	}
	return nil
}

// Get 打开一个对象以供读取。
func (c *COS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.client.Object.Get(ctx, key, nil)
	if err != nil {
		if cos.IsNotFoundError(err) {
			return nil, fmt.Errorf("storage: cos get %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: cos get %q: %w", key, err)
	}
	return resp.Body, nil
}

// Delete 删除一个对象。删除不存在的对象是无操作。
func (c *COS) Delete(ctx context.Context, key string) error {
	if _, err := c.client.Object.Delete(ctx, key, nil); err != nil {
		if cos.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("storage: cos delete %q: %w", key, err)
	}
	return nil
}

// Exists 报告对象是否存在。
func (c *COS) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := c.client.Object.Head(ctx, key, nil); err != nil {
		if cos.IsNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("storage: cos stat %q: %w", key, err)
	}
	return true, nil
}

// Stat 返回已存储对象的元数据。
func (c *COS) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	resp, err := c.client.Object.Head(ctx, key, nil)
	if err != nil {
		if cos.IsNotFoundError(err) {
			return nil, fmt.Errorf("storage: cos stat %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: cos stat %q: %w", key, err)
	}
	return &ObjectInfo{
		Key:          key,
		Size:         parseContentLength(resp.Header),
		ContentType:  resp.Header.Get("Content-Type"),
		LastModified: parseLastModified(resp.Header),
	}, nil
}

// URL 返回对象键对应的公开 URL；私有桶返回一个临时签名 URL。
func (c *COS) URL(key string) string {
	if c.private {
		if signed, err := c.client.Object.GetPresignedURL(
			context.Background(), http.MethodGet, key, c.secretID, c.secretKey, c.presignExpiry, nil,
		); err == nil {
			return signed.String()
		}
	}
	if c.domain != "" {
		return c.domain + "/" + key
	}
	return strings.TrimRight(c.bucketURL.String(), "/") + "/" + key
}

// List 枚举桶中的全部对象，供孤儿对象对账使用。
func (c *COS) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	const pageSize = 1000
	var objects []ObjectInfo
	marker := ""
	for {
		result, _, err := c.client.Bucket.Get(ctx, &cos.BucketGetOptions{
			Prefix:  prefix,
			Marker:  marker,
			MaxKeys: pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("storage: cos list: %w", err)
		}
		for i := range result.Contents {
			objects = append(objects, ObjectInfo{
				Key:          result.Contents[i].Key,
				Size:         result.Contents[i].Size,
				LastModified: parseCOSTime(result.Contents[i].LastModified),
			})
		}
		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
		if marker == "" && len(result.Contents) > 0 {
			marker = result.Contents[len(result.Contents)-1].Key
		}
		if marker == "" {
			break
		}
	}
	return objects, nil
}

// PresignPut 签发一个 COS PUT 签名 URL，客户端用它来直接上传。
func (c *COS) PresignPut(ctx context.Context, key string, opts PresignOptions) (*PresignedRequest, error) {
	expiry := opts.Expires
	if expiry <= 0 {
		expiry = c.presignExpiry
	}
	var presignOpt *cos.PresignedURLOptions
	headers := map[string]string{}
	if opts.ContentType != "" {
		header := http.Header{}
		header.Set("Content-Type", opts.ContentType)
		presignOpt = &cos.PresignedURLOptions{Header: &header}
		headers["Content-Type"] = opts.ContentType
	}
	signed, err := c.client.Object.GetPresignedURL(ctx, http.MethodPut, key, c.secretID, c.secretKey, expiry, presignOpt)
	if err != nil {
		return nil, fmt.Errorf("storage: cos presign: %w", err)
	}
	return &PresignedRequest{
		URL:       signed.String(),
		Method:    http.MethodPut,
		Headers:   headers,
		ExpiresAt: time.Now().Add(expiry),
	}, nil
}

// parseCOSTime 解析 COS 返回的 ISO8601 时间字符串。
func parseCOSTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
