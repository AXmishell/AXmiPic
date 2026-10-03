package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	qiniuauth "github.com/qiniu/go-sdk/v7/auth"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"

	"github.com/AXmishell/axmipic/internal/config"
)

const (
	qiniuDefaultUploadHost = "https://upload.qiniup.com"
	qiniuNoSuchFileCode    = 612
	qiniuServerTokenTTL    = time.Hour
)

// Qiniu stores objects in Qiniu Kodo.
type Qiniu struct {
	mac        *qiniuauth.Credentials
	accessKey  string
	bucket     string
	domain     string
	uploadHost string
	private    bool

	// uploader needs no region; the bucket manager does, so it is resolved
	// lazily (and only once) because it requires a network lookup.
	uploader *qiniustorage.FormUploader
	qcfg     *qiniustorage.Config

	mu      sync.Mutex
	manager *qiniustorage.BucketManager
	zoneID  string
}

// NewQiniu creates a Qiniu Kodo storage driver.
func NewQiniu(cfg config.QiniuConfig) (*Qiniu, error) {
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("storage: qiniu.access_key and qiniu.secret_key must not be empty")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("storage: qiniu.bucket must not be empty")
	}
	if strings.TrimSpace(cfg.Domain) == "" {
		return nil, fmt.Errorf("storage: qiniu.domain must not be empty")
	}
	uploadHost := cfg.UploadHost
	if uploadHost == "" {
		uploadHost = qiniuDefaultUploadHost
	}
	mac := qiniuauth.New(cfg.AccessKey, cfg.SecretKey)
	qcfg := &qiniustorage.Config{UseHTTPS: cfg.UseHTTPS, UpHost: uploadHost}
	q := &Qiniu{
		mac:        mac,
		accessKey:  cfg.AccessKey,
		bucket:     cfg.Bucket,
		domain:     strings.TrimRight(cfg.Domain, "/"),
		uploadHost: uploadHost,
		private:    cfg.Private,
		uploader:   qiniustorage.NewFormUploader(qcfg),
		qcfg:       qcfg,
		zoneID:     cfg.Zone,
	}
	if cfg.Zone != "" {
		region, ok := qiniustorage.GetRegionByID(qiniustorage.RegionID(cfg.Zone))
		if !ok {
			return nil, fmt.Errorf("storage: qiniu.zone %q is unknown", cfg.Zone)
		}
		qcfg.Region = &region
		q.manager = qiniustorage.NewBucketManager(mac, qcfg)
	}
	return q, nil
}

// Put uploads an object.
func (q *Qiniu) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	policy := qiniustorage.PutPolicy{
		Scope:   q.bucket + ":" + key,
		Expires: uint64(time.Now().Add(qiniuServerTokenTTL).Unix()),
	}
	token := policy.UploadToken(q.mac)
	var ret qiniustorage.PutRet
	extra := &qiniustorage.PutExtra{MimeType: contentType}
	if err := q.uploader.Put(ctx, &ret, token, key, r, size, extra); err != nil {
		return fmt.Errorf("storage: qiniu put %q: %w", key, err)
	}
	return nil
}

// Get opens an object for reading.
func (q *Qiniu) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.objectURL(key), nil)
	if err != nil {
		return nil, fmt.Errorf("storage: qiniu get %q: %w", key, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: qiniu get %q: %w", key, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("storage: qiniu get %q: %w", key, ErrNotFound)
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("storage: qiniu get %q: unexpected status %d", key, resp.StatusCode)
	}
}

// Delete removes an object. Deleting a missing object is a no-op.
func (q *Qiniu) Delete(ctx context.Context, key string) error {
	manager, err := q.bucketManager(ctx)
	if err != nil {
		return err
	}
	if err := manager.Delete(q.bucket, key); err != nil {
		if isQiniuNotFound(err) {
			return nil
		}
		return fmt.Errorf("storage: qiniu delete %q: %w", key, err)
	}
	return nil
}

// Exists reports whether an object is present.
func (q *Qiniu) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := q.Stat(ctx, key); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Stat returns metadata for a stored object.
func (q *Qiniu) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	manager, err := q.bucketManager(ctx)
	if err != nil {
		return nil, err
	}
	info, err := manager.Stat(q.bucket, key)
	if err != nil {
		if isQiniuNotFound(err) {
			return nil, fmt.Errorf("storage: qiniu stat %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: qiniu stat %q: %w", key, err)
	}
	return &ObjectInfo{Key: key, Size: info.Fsize, ContentType: info.MimeType}, nil
}

// URL returns the public URL for an object key.
func (q *Qiniu) URL(key string) string {
	return q.domain + "/" + key
}

// PresignPut issues a Qiniu upload token that the client uses to upload
// directly. The policy pins the exact key, size limit, and allowed media types.
func (q *Qiniu) PresignPut(_ context.Context, key string, opts PresignOptions) (*PresignedRequest, error) {
	expiresAt := time.Now().Add(opts.Expires)
	policy := qiniustorage.PutPolicy{
		Scope:      q.bucket + ":" + key,
		Expires:    uint64(expiresAt.Unix()),
		InsertOnly: 1,
		FsizeLimit: opts.MaxSize,
		MimeLimit:  strings.Join(opts.AllowedContentTypes, ";"),
		ReturnBody: `{"key":"$(key)","hash":"$(etag)","fsize":$(fsize),"mimeType":"$(mimeType)"}`,
	}
	token := policy.UploadToken(q.mac)
	return &PresignedRequest{
		URL:       q.uploadHost,
		Method:    http.MethodPost,
		Fields:    map[string]string{"token": token, "key": key},
		ExpiresAt: expiresAt,
	}, nil
}

func (q *Qiniu) objectURL(key string) string {
	if q.private {
		return qiniustorage.MakePrivateURL(q.mac, q.domain, key, time.Now().Add(qiniuServerTokenTTL).Unix())
	}
	return q.domain + "/" + key
}

// bucketManager returns the Qiniu bucket manager, resolving the bucket's region
// on first use when no zone was configured.
func (q *Qiniu) bucketManager(_ context.Context) (*qiniustorage.BucketManager, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.manager != nil {
		return q.manager, nil
	}
	region, err := qiniustorage.GetRegion(q.accessKey, q.bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: qiniu resolve region for bucket %q: %w", q.bucket, err)
	}
	q.qcfg.Region = region
	q.manager = qiniustorage.NewBucketManager(q.mac, q.qcfg)
	return q.manager, nil
}

func isQiniuNotFound(err error) bool {
	var info *qiniustorage.ErrorInfo
	return errors.As(err, &info) && info.Code == qiniuNoSuchFileCode
}
