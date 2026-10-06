// Package storage 定义了 AXmiPic 使用的对象存储抽象。
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound 在请求的对象不存在时返回。
var ErrNotFound = errors.New("storage: object not found")

// ErrInvalidKey 在存储键格式错误或不安全时返回。
var ErrInvalidKey = errors.New("storage: invalid key")

// ObjectInfo 描述已存储对象的元数据。
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	// LastModified 是对象最后修改时间；后端不提供时为零值。
	LastModified time.Time
}

// ObjectLister 由能够枚举对象的后端实现，用于孤儿对象对账。prefix 为空时
// 枚举全部对象。
type ObjectLister interface {
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// Storage 存储并提供图片对象。实现必须支持并发安全使用。键是由应用程序
// 生成的、以斜杠分隔的相对路径，绝不来自用户输入。
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	URL(key string) string
	Stat(ctx context.Context, key string) (*ObjectInfo, error)
}

// PresignOptions 约束直传存储的上传请求。
type PresignOptions struct {
	// ContentType 是客户端必须发送的确切媒体类型。
	ContentType string
	// AllowedContentTypes 供接受媒体类型模式的后端使用。
	AllowedContentTypes []string
	// MaxSize 是可接受的最大对象大小（字节）。
	MaxSize int64
	// Expires 是预签名请求保持有效的时间长度。
	Expires time.Duration
}

// PresignedRequest 描述客户端如何将对象直传到存储后端。
type PresignedRequest struct {
	URL       string
	Method    string
	Fields    map[string]string
	Headers   map[string]string
	ExpiresAt time.Time
}

// Presigner 由支持客户端直传的后端实现。不支持的后端（例如本地存储）通过
// 类型断言检测，并报告为不支持。
type Presigner interface {
	PresignPut(ctx context.Context, key string, opts PresignOptions) (*PresignedRequest, error)
}
