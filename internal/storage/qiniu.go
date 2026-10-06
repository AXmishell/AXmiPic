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

// Qiniu 将对象存储在七牛云 Kodo 中。
type Qiniu struct {
	mac        *qiniuauth.Credentials
	accessKey  string
	bucket     string
	domain     string
	uploadHost string
	private    bool

	// uploader 不需要 region；而 bucket manager 需要，因此它在首次使用时
	// 惰性解析（且仅解析一次），因为该操作需要网络查询。
	uploader *qiniustorage.FormUploader
	qcfg     *qiniustorage.Config

	mu      sync.Mutex
	manager *qiniustorage.BucketManager
	zoneID  string
}

// NewQiniu 创建一个七牛云 Kodo 存储驱动。
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

// Put 上传一个对象。
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

// Get 打开一个对象以供读取。
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

// Delete 删除一个对象。删除不存在的对象是无操作。
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

// Exists 报告对象是否存在。
func (q *Qiniu) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := q.Stat(ctx, key); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Stat 返回已存储对象的元数据。
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

// URL 返回对象键对应的公开 URL。
func (q *Qiniu) URL(key string) string {
	return q.domain + "/" + key
}

// List 枚举七牛桶中的全部对象，供孤儿对象对账使用。
func (q *Qiniu) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	manager, err := q.bucketManager(ctx)
	if err != nil {
		return nil, err
	}
	const pageSize = 1000
	var objects []ObjectInfo
	marker := ""
	for {
		entries, _, next, hasNext, listErr := manager.ListFiles(q.bucket, prefix, "", marker, pageSize)
		if listErr != nil {
			return nil, fmt.Errorf("storage: qiniu list: %w", listErr)
		}
		for i := range entries {
			// 七牛的 PutTime 以 100 纳秒为单位。
			modified := time.Unix(0, entries[i].PutTime*100).UTC()
			objects = append(objects, ObjectInfo{
				Key:          entries[i].Key,
				Size:         entries[i].Fsize,
				ContentType:  entries[i].MimeType,
				LastModified: modified,
			})
		}
		if !hasNext || next == "" {
			break
		}
		marker = next
	}
	return objects, nil
}

// PresignPut 签发一个七牛上传凭证，客户端用它来直接上传。
// 该策略固定确切的键、大小限制和允许的媒体类型。
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

// bucketManager 返回七牛 bucket manager，在未配置 zone 时于首次使用时
// 解析桶所在的 region。
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
