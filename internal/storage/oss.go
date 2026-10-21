package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/AXmishell/axmipic/internal/config"
)

// ossDefaultPresignExpiry 是未配置 presign_expiry_sec 时的签名有效期。
const ossDefaultPresignExpiry = 15 * time.Minute

// OSS 将对象存储在阿里云对象存储 OSS 中。
type OSS struct {
	bucket        *oss.Bucket
	bucketName    string
	endpointHost  string
	domain        string
	private       bool
	secure        bool
	presignExpiry time.Duration
}

// NewOSS 创建一个阿里云 OSS 存储驱动。
func NewOSS(cfg config.OSSConfig) (*OSS, error) {
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	bucketName := strings.TrimSpace(cfg.Bucket)
	if accessKeyID == "" || accessKeySecret == "" {
		return nil, fmt.Errorf("storage: oss.access_key_id and oss.access_key_secret must not be empty")
	}
	if bucketName == "" {
		return nil, fmt.Errorf("storage: oss.bucket must not be empty")
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		region := strings.TrimSpace(cfg.Region)
		if region == "" {
			return nil, fmt.Errorf("storage: oss.region or oss.endpoint must not be empty")
		}
		// 同时接受 cn-hangzhou 与 oss-cn-hangzhou 两种写法。
		region = strings.TrimPrefix(region, "oss-")
		endpoint = "https://oss-" + region + ".aliyuncs.com"
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "https"
		if !cfg.UseHTTPS {
			scheme = "http"
		}
		endpoint = scheme + "://" + endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")
	secure := strings.HasPrefix(endpoint, "https://")

	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("storage: oss client: %w", err)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("storage: oss bucket %q: %w", bucketName, err)
	}

	expiry := time.Duration(cfg.PresignExpirySec) * time.Second
	if expiry <= 0 {
		expiry = ossDefaultPresignExpiry
	}

	return &OSS{
		bucket:        bucket,
		bucketName:    bucketName,
		endpointHost:  strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://"), "/"),
		domain:        strings.TrimRight(strings.TrimSpace(cfg.Domain), "/"),
		private:       cfg.Private,
		secure:        secure,
		presignExpiry: expiry,
	}, nil
}

// Put 上传一个对象。
func (o *OSS) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opts := make([]oss.Option, 0, 2)
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}
	if size > 0 {
		opts = append(opts, oss.ContentLength(size))
	}
	if err := o.bucket.PutObject(key, r, opts...); err != nil {
		return fmt.Errorf("storage: oss put %q: %w", key, err)
	}
	return nil
}

// Get 打开一个对象以供读取。
func (o *OSS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	body, err := o.bucket.GetObject(key)
	if err != nil {
		if isOSSNotFound(err) {
			return nil, fmt.Errorf("storage: oss get %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: oss get %q: %w", key, err)
	}
	return body, nil
}

// Delete 删除一个对象。删除不存在的对象是无操作。
func (o *OSS) Delete(ctx context.Context, key string) error {
	if err := o.bucket.DeleteObject(key); err != nil {
		if isOSSNotFound(err) {
			return nil
		}
		return fmt.Errorf("storage: oss delete %q: %w", key, err)
	}
	return nil
}

// Exists 报告对象是否存在。
func (o *OSS) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := o.bucket.IsObjectExist(key)
	if err != nil {
		return false, fmt.Errorf("storage: oss stat %q: %w", key, err)
	}
	return exists, nil
}

// Stat 返回已存储对象的元数据。
func (o *OSS) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	header, err := o.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		if isOSSNotFound(err) {
			return nil, fmt.Errorf("storage: oss stat %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: oss stat %q: %w", key, err)
	}
	return &ObjectInfo{
		Key:          key,
		Size:         parseContentLength(header),
		ContentType:  header.Get("Content-Type"),
		LastModified: parseLastModified(header),
	}, nil
}

// URL 返回对象键对应的公开 URL；私有桶返回一个临时签名 URL。
func (o *OSS) URL(key string) string {
	if o.private {
		if signed, err := o.bucket.SignURL(key, oss.HTTPGet, int64(o.presignExpiry.Seconds())); err == nil {
			return signed
		}
	}
	if o.domain != "" {
		return o.domain + "/" + key
	}
	scheme := "https"
	if !o.secure {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.%s/%s", scheme, o.bucketName, o.endpointHost, key)
}

// List 枚举桶中的全部对象，供孤儿对象对账使用。
func (o *OSS) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	const pageSize = 1000
	var objects []ObjectInfo
	marker := ""
	for {
		result, err := o.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(pageSize))
		if err != nil {
			return nil, fmt.Errorf("storage: oss list: %w", err)
		}
		for i := range result.Objects {
			objects = append(objects, ObjectInfo{
				Key:          result.Objects[i].Key,
				Size:         result.Objects[i].Size,
				LastModified: result.Objects[i].LastModified,
			})
		}
		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
		if marker == "" && len(result.Objects) > 0 {
			marker = result.Objects[len(result.Objects)-1].Key
		}
		if marker == "" {
			break
		}
	}
	return objects, nil
}

// PresignPut 签发一个 OSS PUT 签名 URL，客户端用它来直接上传。
func (o *OSS) PresignPut(ctx context.Context, key string, opts PresignOptions) (*PresignedRequest, error) {
	expiresAt := time.Now().Add(opts.Expires)
	seconds := int64(opts.Expires.Seconds())
	if seconds <= 0 {
		seconds = int64(o.presignExpiry.Seconds())
		expiresAt = time.Now().Add(o.presignExpiry)
	}
	signed, err := o.bucket.SignURL(key, oss.HTTPPut, seconds)
	if err != nil {
		return nil, fmt.Errorf("storage: oss presign: %w", err)
	}
	headers := map[string]string{}
	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}
	return &PresignedRequest{
		URL:       signed,
		Method:    http.MethodPut,
		Headers:   headers,
		ExpiresAt: expiresAt,
	}, nil
}

// isOSSNotFound 判断 OSS 错误是否为对象不存在。
func isOSSNotFound(err error) bool {
	var serviceErr oss.ServiceError
	return errors.As(err, &serviceErr) && serviceErr.StatusCode == http.StatusNotFound
}

// parseContentLength 从响应头解析对象大小。
func parseContentLength(header http.Header) int64 {
	size, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	if err != nil {
		return 0
	}
	return size
}

// parseLastModified 从响应头解析最后修改时间。
func parseLastModified(header http.Header) time.Time {
	value := header.Get("Last-Modified")
	if value == "" {
		return time.Time{}
	}
	modified, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}
	}
	return modified
}
