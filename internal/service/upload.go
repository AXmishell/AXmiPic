// Package service 包含 AXmiPic 的业务逻辑。
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/moderation"
	"github.com/AXmishell/axmipic/internal/security"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrFileTooLarge 在上传超过配置的大小限制时返回。
var ErrFileTooLarge = errors.New("service: file exceeds the maximum allowed size")

// ErrUnsupportedType 在上传的媒体类型不被允许时返回。
var ErrUnsupportedType = errors.New("service: unsupported media type")

// ErrPresignUnsupported 在当前存储后端无法签发预签名直传请求时返回。
var ErrPresignUnsupported = errors.New("service: presigned upload is not supported by the configured storage driver")

// ErrInvalidInput 在请求格式错误时返回。
var ErrInvalidInput = errors.New("service: invalid input")

// ErrQuotaExceeded 在上传会超出所有者配额时返回。
var ErrQuotaExceeded = errors.New("service: storage quota exceeded")

// ErrGuestQuotaExceeded 在匿名访客的按 IP 窗口配额已用尽时返回。
var ErrGuestQuotaExceeded = errors.New("service: guest ip quota exceeded")

// ErrForbidden 在调用者无权访问某个资源时返回。
var ErrForbidden = errors.New("service: forbidden")

// ErrContentBlocked 在上传内容未通过安全扫描时返回。
var ErrContentBlocked = errors.New("service: content rejected by security scan")

// ErrNotFound 在请求的图片不存在时返回。
var ErrNotFound = store.ErrNotFound

// dimensionProbeLimit 限定了为解码图片尺寸而读取的对象大小上限，从而在
// 确认时避免完整下载。
const dimensionProbeLimit = 1 << 20

// mimeExtensions 将可接受的 MIME 类型映射到磁盘上使用的键扩展名。
var mimeExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// extensionForMIME 返回某个媒体类型对应的存储键扩展名，优先使用内置表，
// 再回退到系统 MIME 数据库，这样即使某个被允许的类型不在表中也能获得可用的
// 扩展名。
func extensionForMIME(mimeType string) string {
	if ext, ok := mimeExtensions[mimeType]; ok {
		return ext
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// UploadPolicy 约束 UploadService 接受的内容。
type UploadPolicy struct {
	MaxSizeBytes     int64
	AllowedMIMETypes []string
	PresignExpiry    time.Duration
}

// UploadLimits 是解析后的、针对某个调用方生效的上传限制。
type UploadLimits struct {
	MaxSizeBytes     int64
	AllowedMIMETypes []string
}

// UploadPolicyResolver 按调用方解析生效的上传限制（例如角色组策略）。
// 解析失败时调用方会拒绝上传，避免悄然放宽限制。
type UploadPolicyResolver interface {
	UploadLimitsFor(ctx context.Context, principal *auth.Principal) (UploadLimits, error)
}

// UploadInput 是一个已准备好被持久化的上传请求。
type UploadInput struct {
	Data     []byte
	MimeType string
	// OriginalName 是上传时的原始文件名，可为空。
	OriginalName string
}

// PresignInput 请求为某份内容提供预签名直传。
type PresignInput struct {
	MimeType string
	Size     int64
}

// PresignResult 告知客户端如何将对象直接上传到存储。
type PresignResult struct {
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ImageDTO 是已存储图片在 API 中的表示形式。
type ImageDTO struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	StorageID string `json:"storage_id,omitempty"`
	// AlbumID 为所属相册 id；为空表示未归入任何相册。
	AlbumID string `json:"album_id,omitempty"`
	// Permission 为图片可见性：public（可出现在图片广场）或 private（默认）。
	Permission string `json:"permission"`
	// OwnerUsername 为图片所有者的用户名，仅在公开列表（广场/公开相册）中填充。
	OwnerUsername string `json:"owner_username,omitempty"`
	// OriginalName 为上传时的原始文件名；Filename 为重命名后的存储文件名；
	// Hash 为内容 sha256 十六进制摘要。
	OriginalName string `json:"original_name"`
	Filename     string `json:"filename"`
	Hash         string `json:"hash"`
	URL          string `json:"url"`
	// Thumbnail 是为列表展示准备的缩略图 URL，始终经过本实例的即时处理端点，
	// 因此对本地/S3/七牛等所有后端都生效。
	Thumbnail string    `json:"thumbnail"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	CreatedAt time.Time `json:"created_at"`
}

// ImageFilter 约束图片列表查询。
type ImageFilter struct {
	// Order 为排序方式：newest（默认）、earliest、largest、smallest。
	Order string
	// Keyword 按原文件名、存储文件名或键模糊匹配。
	Keyword string
	// AlbumID 非空时仅返回属于该相册的图片。
	AlbumID *string
	// Permission 非空时仅返回该可见性的图片（public/private）。
	Permission string
	// UserID 非空时仅返回该用户的图片，用于图片广场按作者过滤。
	UserID string
	// Cursor 为不透明的 keyset 游标；非空时优先于 page 进行翻页。
	Cursor string
}

// ListResult 是图片的分页集合。
type ListResult struct {
	Items    []ImageDTO `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	// NextCursor 非空时可用于请求下一页（keyset 游标）。仅在按时间或大小排序
	// 且本页已满时返回。
	NextCursor string `json:"next_cursor,omitempty"`
}

// UploadService 协调校验、去重、存储和元数据。
type UploadService struct {
	repo     *store.Repository
	manager  *storage.Manager
	policy   UploadPolicy
	resolver UploadPolicyResolver
	scanner  security.Scanner

	// policyMu 保护 policy，使其可在运行时热替换。
	policyMu sync.RWMutex

	// mediaBaseURL 是构造缩略图/转换 URL 时使用的实例公开根地址。
	mediaBaseURL string

	// guestIPQuotaBytes 与 guestIPQuotaWindow 控制匿名访客按 IP 的累计上传配额；
	// guestIPQuotaBytes<=0 表示不限。
	guestIPQuotaBytes  int64
	guestIPQuotaWindow time.Duration

	// moderator 为图片广场的 AI 审查器；非 nil 时，图片在设为公开前先审查。
	// moderationMaxBytes>0 时，超过该体积的图片按审查不通过处理。
	moderator          moderation.Moderator
	moderationMaxBytes int64
}

// NewUploadService 构造一个 UploadService。
func NewUploadService(repo *store.Repository, manager *storage.Manager, policy UploadPolicy) *UploadService {
	return &UploadService{repo: repo, manager: manager, policy: policy}
}

// SetPolicyResolver 安装一个按调用方解析上传限制的解析器（例如角色组策略）。
func (s *UploadService) SetPolicyResolver(resolver UploadPolicyResolver) {
	s.resolver = resolver
}

// SetPolicy 在运行时替换全局上传策略（后台设置热更新）。
func (s *UploadService) SetPolicy(policy UploadPolicy) {
	s.policyMu.Lock()
	s.policy = policy
	s.policyMu.Unlock()
}

// SetMediaBaseURL 设置实例公开根地址，用于构造列表缩略图 URL。
func (s *UploadService) SetMediaBaseURL(baseURL string) {
	s.mediaBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// SetScanner 安装一个上传内容安全扫描器。
func (s *UploadService) SetScanner(scanner security.Scanner) {
	s.scanner = scanner
}

// SetModerator 安装图片广场的 AI 审查器。maxReviewBytes>0 时，体积超过该值的
// 图片被视为审查不通过。传 nil 可关闭审查。
func (s *UploadService) SetModerator(m moderation.Moderator, maxReviewBytes int64) {
	s.moderator = m
	s.moderationMaxBytes = maxReviewBytes
}

// ModerationEnabled 报告图片广场 AI 审查是否已启用。
func (s *UploadService) ModerationEnabled() bool {
	return s.moderator != nil
}

// SetGuestIPQuota 配置匿名访客按客户端 IP 的累计上传配额（固定窗口内）。
// limitBytes<=0 表示不限。
func (s *UploadService) SetGuestIPQuota(limitBytes int64, window time.Duration) {
	s.guestIPQuotaBytes = limitBytes
	s.guestIPQuotaWindow = window
}

// clientIPKey 是上下文键，用于把客户端 IP 传递给访客配额记账。
type clientIPKey struct{}

// WithClientIP 把客户端 IP 附带到上下文，供访客 IP 配额记账使用。
func WithClientIP(ctx context.Context, ip string) context.Context {
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func clientIPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// reserveGuestIPQuota 为匿名访客预留按 IP 的窗口配额。非访客或未配置时无操作。
func (s *UploadService) reserveGuestIPQuota(ctx context.Context, principal *auth.Principal, amount int64) (func(), bool, error) {
	if s.guestIPQuotaBytes <= 0 || !principal.IsGuest() {
		return func() {}, true, nil
	}
	ip := clientIPFrom(ctx)
	if ip == "" {
		return func() {}, true, nil
	}
	ok, err := s.repo.ReserveGuestIPQuota(ctx, ip, amount, s.guestIPQuotaBytes, s.guestIPQuotaWindow)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return func() {}, false, nil
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		_ = s.repo.ReleaseGuestIPQuota(context.WithoutCancel(ctx), ip, amount)
	}, true, nil
}

// ScannerName 返回当前扫描器名称；未配置时返回 "none"。
func (s *UploadService) ScannerName() string {
	if s.scanner == nil {
		return "none"
	}
	return s.scanner.Name()
}

// scanContent 对上传内容执行安全扫描；未配置扫描器时直接放行。
func (s *UploadService) scanContent(ctx context.Context, data []byte, mimeType string) error {
	if s.scanner == nil {
		return nil
	}
	result, err := s.scanner.Scan(ctx, data, mimeType)
	if err != nil {
		return fmt.Errorf("upload: security scan: %w", err)
	}
	if result.IsBlocked() {
		return fmt.Errorf("%w: %s", ErrContentBlocked, result.Reason)
	}
	return nil
}

// policyFor 解析某个主体生效上传策略，并返回其允许的媒体类型集合。
func (s *UploadService) policyFor(ctx context.Context, principal *auth.Principal) (UploadPolicy, map[string]struct{}, error) {
	s.policyMu.RLock()
	policy := s.policy
	s.policyMu.RUnlock()
	if s.resolver != nil {
		limits, err := s.resolver.UploadLimitsFor(ctx, principal)
		if err != nil {
			return UploadPolicy{}, nil, fmt.Errorf("resolve upload policy: %w", err)
		}
		if limits.MaxSizeBytes > 0 {
			policy.MaxSizeBytes = limits.MaxSizeBytes
		}
		if len(limits.AllowedMIMETypes) > 0 {
			policy.AllowedMIMETypes = limits.AllowedMIMETypes
		}
	}
	allowed := make(map[string]struct{}, len(policy.AllowedMIMETypes))
	for _, mimeType := range policy.AllowedMIMETypes {
		allowed[mimeType] = struct{}{}
	}
	return policy, allowed, nil
}

// backendFor 返回某张图片所在的存储后端：优先按记录的 storage_id 解析，
// 为空或已删除时回退到当前默认后端。
func (s *UploadService) backendFor(image *store.Image) storage.Storage {
	if image.StorageID != nil {
		return s.manager.Resolve(*image.StorageID)
	}
	return s.manager.Current()
}

// urlFor 返回对象在指定后端上的公开 URL。
func (s *UploadService) urlFor(backend storage.Storage, key string) string {
	if backend == nil {
		return ""
	}
	return backend.URL(key)
}

// BackendForKey 返回存储指定键图片的后端，供 API 层流式读取对象时路由。
// 若数据库中没有该键的记录，则回退到当前默认后端。
func (s *UploadService) BackendForKey(ctx context.Context, key string) (storage.Storage, error) {
	image, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return s.manager.Current(), nil
		}
		return nil, err
	}
	return s.backendFor(image), nil
}

// Upload 校验、存储并记录一张归 principal 所有的图片。存储键为随机、不可猜测
// 的路径；相同所有者上传相同内容时按内容哈希去重，不同所有者各自持有独立对象，
// 避免跨用户泄露与删除耦合。
func (s *UploadService) Upload(ctx context.Context, principal *auth.Principal, in UploadInput) (*ImageDTO, error) {
	policy, allowed, err := s.policyFor(ctx, principal)
	if err != nil {
		return nil, err
	}
	size := int64(len(in.Data))
	if size > policy.MaxSizeBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, size, policy.MaxSizeBytes)
	}
	if _, ok := allowed[in.MimeType]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, in.MimeType)
	}
	if err := s.scanContent(ctx, in.Data, in.MimeType); err != nil {
		return nil, err
	}
	hash := contentHash(in.Data)
	ownerID := ownerIDOf(principal)
	ownerKey := stringValue(ownerID)

	// 同一所有者的相同内容直接复用已有记录。
	existing, err := s.repo.GetByHashAndUser(ctx, hash, ownerKey)
	if err == nil {
		return toDTO(existing, s.mediaBaseURL), nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("upload: lookup existing image: %w", err)
	}

	release, ok, err := s.reserveQuota(ctx, principal, ownerID, size)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrQuotaExceeded
	}
	releaseIP, ok, err := s.reserveGuestIPQuota(ctx, principal, size)
	if err != nil {
		release()
		return nil, err
	}
	if !ok {
		release()
		return nil, ErrGuestQuotaExceeded
	}
	committed := false
	defer func() {
		if !committed {
			releaseIP()
			release()
		}
	}()

	backend := s.manager.Current()
	if backend == nil {
		return nil, fmt.Errorf("%w: no storage backend configured", ErrStorageConfig)
	}
	currentID := s.manager.CurrentID()
	key := randomKey(in.MimeType)
	if err := backend.Put(ctx, key, bytes.NewReader(in.Data), size, in.MimeType); err != nil {
		return nil, fmt.Errorf("upload: store object: %w", err)
	}

	width, height := decodeDimensions(in.Data)
	image := &store.Image{
		ID:           uuid.NewString(),
		Key:          key,
		UserID:       ownerID,
		StorageID:    storageIDPtr(currentID),
		OriginalName: sanitizeOriginalName(in.OriginalName),
		Filename:     path.Base(key),
		Hash:         hash,
		URL:          s.urlFor(backend, key),
		Size:         size,
		MimeType:     in.MimeType,
		Width:        width,
		Height:       height,
	}
	if err := s.repo.Create(ctx, image); err != nil {
		// 并发上传相同内容可能已先插入记录；此时删除我们刚写入的独立对象并复用
		// 已有记录。
		_ = backend.Delete(context.WithoutCancel(ctx), key)
		if concurrent, getErr := s.repo.GetByHashAndUser(ctx, hash, ownerKey); getErr == nil {
			return toDTO(concurrent, s.mediaBaseURL), nil
		}
		return nil, fmt.Errorf("upload: record image: %w", err)
	}
	committed = true
	return toDTO(image, s.mediaBaseURL), nil
}

// Presign 校验直传请求，从存储后端签发一个预签名请求，并记录一条待确认的
// 上传供后续确认。
func (s *UploadService) Presign(ctx context.Context, principal *auth.Principal, in PresignInput) (*PresignResult, error) {
	presigner := s.manager.PresignerFor("")
	if presigner == nil {
		return nil, ErrPresignUnsupported
	}
	policy, allowed, err := s.policyFor(ctx, principal)
	if err != nil {
		return nil, err
	}
	if _, ok := allowed[in.MimeType]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, in.MimeType)
	}
	if in.Size <= 0 {
		return nil, fmt.Errorf("%w: size must be positive", ErrInvalidInput)
	}
	if in.Size > policy.MaxSizeBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, in.Size, policy.MaxSizeBytes)
	}

	backend := s.manager.Current()
	key := randomKey(in.MimeType)
	req, err := presigner.PresignPut(ctx, key, storage.PresignOptions{
		ContentType:         in.MimeType,
		AllowedContentTypes: policy.AllowedMIMETypes,
		MaxSize:             policy.MaxSizeBytes,
		Expires:             policy.PresignExpiry,
	})
	if err != nil {
		return nil, fmt.Errorf("presign: %w", err)
	}

	pending := &store.PendingUpload{
		Key:       key,
		UserID:    ownerIDOf(principal),
		StorageID: storageIDPtr(s.manager.CurrentID()),
		MimeType:  in.MimeType,
		MaxSize:   policy.MaxSizeBytes,
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.repo.CreatePendingUpload(ctx, pending); err != nil {
		return nil, fmt.Errorf("presign: record pending upload: %w", err)
	}

	return &PresignResult{
		Key:       key,
		URL:       s.urlFor(backend, key),
		UploadURL: req.URL,
		Method:    req.Method,
		Fields:    req.Fields,
		Headers:   req.Headers,
		ExpiresAt: req.ExpiresAt,
	}, nil
}

// Confirm 验证某个预签名对象已上传，并记录其元数据。对于已记录的键，
// 它是幂等的。
func (s *UploadService) Confirm(ctx context.Context, principal *auth.Principal, key string) (*ImageDTO, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("%w: key must not be empty", ErrInvalidInput)
	}
	if existing, err := s.repo.GetByKey(ctx, key); err == nil {
		// 幂等：该对象已入库时直接返回记录，但仅限其所有者（或管理员）。
		if !canAccess(principal, existing.UserID) {
			return nil, ErrForbidden
		}
		return toDTO(existing, s.mediaBaseURL), nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("confirm: lookup existing image: %w", err)
	}

	policy, allowed, err := s.policyFor(ctx, principal)
	if err != nil {
		return nil, err
	}

	pending, err := s.repo.GetPendingUpload(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("confirm: no pending upload for key: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("confirm: lookup pending upload: %w", err)
	}
	if time.Now().After(pending.ExpiresAt) {
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("confirm: pending upload expired: %w", ErrNotFound)
	}
	if !canAccess(principal, pending.UserID) {
		return nil, ErrForbidden
	}

	backend := s.manager.Resolve(storageIDValue(pending.StorageID))
	info, err := backend.Stat(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("confirm: object was not uploaded: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("confirm: stat object: %w", err)
	}
	if info.Size > policy.MaxSizeBytes {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: object is %d bytes", ErrFileTooLarge, info.Size)
	}
	if _, ok := allowed[info.ContentType]; !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, info.ContentType)
	}
	// 验证对象实际上看起来像一张被允许的图片，而不是仅凭客户端声明的
	// 内容类型来信任。
	detected, width, height := s.probeObject(ctx, backend, key)
	if _, ok := allowed[detected]; !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: object content is %q", ErrUnsupportedType, detected)
	}
	// 对直传对象做安全扫描；命中则删除对象并拒绝。
	if s.scanner != nil {
		object, getErr := backend.Get(ctx, key)
		if getErr != nil {
			return nil, fmt.Errorf("confirm: security scan read: %w", getErr)
		}
		probe, readErr := io.ReadAll(io.LimitReader(object, dimensionProbeLimit))
		_ = object.Close()
		if readErr != nil {
			return nil, fmt.Errorf("confirm: security scan: %w", readErr)
		}
		if scanErr := s.scanContent(ctx, probe, detected); scanErr != nil {
			_ = backend.Delete(ctx, key)
			_ = s.repo.DeletePendingUpload(ctx, key)
			return nil, scanErr
		}
	}

	ownerID := pending.UserID
	release, ok, err := s.reserveQuota(ctx, principal, ownerID, info.Size)
	if err != nil {
		return nil, err
	}
	if !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, ErrQuotaExceeded
	}
	releaseIP, ok, err := s.reserveGuestIPQuota(ctx, principal, info.Size)
	if err != nil {
		release()
		return nil, err
	}
	if !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		release()
		return nil, ErrGuestQuotaExceeded
	}
	committed := false
	defer func() {
		if !committed {
			releaseIP()
			release()
		}
	}()

	image := &store.Image{
		ID:        uuid.NewString(),
		Key:       key,
		UserID:    ownerID,
		StorageID: pending.StorageID,
		URL:       s.urlFor(backend, key),
		Size:      info.Size,
		MimeType:  detected,
		Width:     width,
		Height:    height,
	}
	if err := s.repo.Create(ctx, image); err != nil {
		if concurrent, getErr := s.repo.GetByKey(ctx, key); getErr == nil {
			_ = s.repo.DeletePendingUpload(ctx, key)
			return toDTO(concurrent, s.mediaBaseURL), nil
		}
		return nil, fmt.Errorf("confirm: record image: %w", err)
	}
	_ = s.repo.DeletePendingUpload(ctx, key)
	committed = true
	return toDTO(image, s.mediaBaseURL), nil
}

// CleanupExpired 移除截至 now 已过期的待确认上传，并删除其未确认的存储
// 对象。它返回被移除的存储对象数量。
func (s *UploadService) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	pending, err := s.repo.ExpiredPendingUploads(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("cleanup: list expired pending uploads: %w", err)
	}
	removed := 0
	for i := range pending {
		key := pending[i].Key
		if _, err := s.repo.GetByKey(ctx, key); err == nil {
			// 该上传终究已被确认；只删除过期的行。
			if delErr := s.repo.DeletePendingUpload(ctx, key); delErr != nil {
				continue
			}
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			continue
		}
		backend := s.manager.Resolve(storageIDValue(pending[i].StorageID))
		if err := backend.Delete(ctx, key); err != nil {
			continue
		}
		if err := s.repo.DeletePendingUpload(ctx, key); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}

// List 返回对 principal 可见的一页图片：管理员可见全部图片，其余人仅可见
// 自己拥有的图片。filter 控制排序、关键字、相册与可见性过滤。
func (s *UploadService) List(ctx context.Context, principal *auth.Principal, page, pageSize int, filter ImageFilter) (*ListResult, error) {
	page, pageSize = normalizePagination(page, pageSize)

	if !principal.IsAdmin() {
		if principal.IsGuest() {
			return &ListResult{Items: []ImageDTO{}, Total: 0, Page: page, PageSize: pageSize}, nil
		}
		return s.listImages(ctx, store.ImageListOptions{
			UserID:     principal.UserID,
			Offset:     (page - 1) * pageSize,
			Limit:      pageSize,
			Order:      filter.Order,
			Keyword:    strings.TrimSpace(filter.Keyword),
			Permission: filter.Permission,
			AlbumID:    filter.AlbumID,
			Cursor:     decodeImageCursor(filter.Cursor),
		}, page, pageSize)
	}
	return s.listImages(ctx, store.ImageListOptions{
		Offset:     (page - 1) * pageSize,
		Limit:      pageSize,
		Order:      filter.Order,
		Keyword:    strings.TrimSpace(filter.Keyword),
		Permission: filter.Permission,
		AlbumID:    filter.AlbumID,
		Cursor:     decodeImageCursor(filter.Cursor),
	}, page, pageSize)
}

// ListPlaza 返回一页公开图片（permission=public），跨所有用户，用于图片广场。
func (s *UploadService) ListPlaza(ctx context.Context, page, pageSize int, filter ImageFilter) (*ListResult, error) {
	page, pageSize = normalizePagination(page, pageSize)
	return s.listImages(ctx, store.ImageListOptions{
		Permission: store.PermissionPublic,
		UserID:     strings.TrimSpace(filter.UserID),
		Offset:     (page - 1) * pageSize,
		Limit:      pageSize,
		Order:      filter.Order,
		Keyword:    strings.TrimSpace(filter.Keyword),
		AlbumID:    filter.AlbumID,
		Cursor:     decodeImageCursor(filter.Cursor),
	}, page, pageSize)
}

// ListAlbumImages 返回相册中的图片，用于公开相册浏览。相册非公开时仅所有者
// 或管理员可访问。
func (s *UploadService) ListAlbumImages(ctx context.Context, principal *auth.Principal, albumID string, page, pageSize int) (*ListResult, error) {
	album, err := s.repo.GetAlbumByID(ctx, albumID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, fmt.Errorf("list album images: %w", err)
	}
	if album.Permission != store.PermissionPublic && !canAccess(principal, album.UserID) {
		return nil, ErrForbidden
	}
	page, pageSize = normalizePagination(page, pageSize)
	return s.listImages(ctx, store.ImageListOptions{
		AlbumID: &albumID,
		Offset:  (page - 1) * pageSize,
		Limit:   pageSize,
	}, page, pageSize)
}

// listImages 执行列表查询并把结果转换为 DTO，同时批量填充所有者用户名。
func (s *UploadService) listImages(ctx context.Context, opts store.ImageListOptions, page, pageSize int) (*ListResult, error) {
	images, total, err := s.repo.ListImages(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	items := make([]ImageDTO, 0, len(images))
	ownerIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range images {
		items = append(items, *toDTO(&images[i], s.mediaBaseURL))
		if images[i].UserID != nil {
			if _, ok := seen[*images[i].UserID]; !ok {
				seen[*images[i].UserID] = struct{}{}
				ownerIDs = append(ownerIDs, *images[i].UserID)
			}
		}
	}
	if len(ownerIDs) > 0 {
		if names, nameErr := s.repo.UsernamesByIDs(ctx, ownerIDs); nameErr == nil {
			for i := range images {
				if images[i].UserID != nil {
					items[i].OwnerUsername = names[*images[i].UserID]
				}
			}
		}
	}
	result := &ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}
	// 本页已满时提供下一页游标，供 keyset 翻页使用。
	if opts.Limit > 0 && len(images) == opts.Limit {
		last := &images[len(images)-1]
		result.NextCursor = encodeImageCursor(normalizeOrder(opts.Order), last)
	}
	return result, nil
}

// normalizeOrder 把排序标识收敛到白名单中的一个已知取值，与存储层保持一致。
func normalizeOrder(order string) string {
	switch order {
	case "earliest", "largest", "smallest":
		return order
	default:
		return "newest"
	}
}

// encodeImageCursor 把一页的最后一条记录编码为不透明游标。时间以 RFC3339Nano
// 编码，从而保留原始时区偏移——数据库按文本比较时间时需要它。
func encodeImageCursor(order string, image *store.Image) string {
	raw := fmt.Sprintf("%s|%s|%d|%s", order, image.CreatedAt.Format(time.RFC3339Nano), image.Size, image.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeImageCursor 解析由 encodeImageCursor 生成的不透明游标；无效时返回 nil，
// 由调用方回退到基于页码的翻页。
func decodeImageCursor(raw string) *store.ImageCursor {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	parts := strings.Split(string(data), "|")
	if len(parts) != 4 {
		return nil
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return nil
	}
	size, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil
	}
	if parts[3] == "" {
		return nil
	}
	return &store.ImageCursor{
		Order:     parts[0],
		CreatedAt: createdAt,
		Size:      size,
		ID:        parts[3],
	}
}

// normalizePagination 将页码与每页数量收敛到合理范围。
func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if page > 1_000_000 {
		page = 1_000_000
	}
	return page, pageSize
}

// Rename 修改一张图片的原文件名（展示名），保留其存储键、存储文件名与哈希
// 不变。所有权校验与直传、删除逻辑一致。
func (s *UploadService) Rename(ctx context.Context, principal *auth.Principal, id, name string) (*ImageDTO, error) {
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("rename image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return nil, ErrForbidden
	}
	clean := sanitizeOriginalName(name)
	if clean == "" {
		return nil, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}
	updated, err := s.repo.RenameImage(ctx, id, clean)
	if err != nil {
		return nil, fmt.Errorf("rename image: %w", err)
	}
	return toDTO(updated, s.mediaBaseURL), nil
}

// Get 按 id 返回单张图片，并强制校验所有权。
func (s *UploadService) Get(ctx context.Context, principal *auth.Principal, id string) (*ImageDTO, error) {
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return nil, ErrForbidden
	}
	return toDTO(image, s.mediaBaseURL), nil
}

// GetByKey 按其存储键返回单张图片。公开服务不强制校验所有权。
func (s *UploadService) GetByKey(ctx context.Context, key string) (*ImageDTO, error) {
	image, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get image by key: %w", err)
	}
	return toDTO(image, s.mediaBaseURL), nil
}

// GetByKeyWithBackend 按其存储键返回图片及其所在的存储后端。它把元数据读取与
// 后端路由合并为一次数据库查询，供 /i/* 热路径使用。
func (s *UploadService) GetByKeyWithBackend(ctx context.Context, key string) (*ImageDTO, storage.Storage, error) {
	image, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("get image by key: %w", err)
	}
	return toDTO(image, s.mediaBaseURL), s.backendFor(image), nil
}

// PermissionResult 描述一次批量可见性变更的结果。
type PermissionResult struct {
	// Published 为实际设为目标可见性（公开）的图片数量。
	Published int `json:"published"`
	// Blocked 为被 AI 审查拦下、保持私有的图片 id。
	Blocked []string `json:"blocked,omitempty"`
	// Reasons 记录被拦图片的判定原因（图片 id -> 原因）。
	Reasons map[string]string `json:"reasons,omitempty"`
}

// SetPermission 批量设置图片的可见性。会先校验全部图片都归 principal 所有，
// 避免越权修改他人图片。当目标为公开且启用了 AI 审查时，每张图片会先经视觉
// 模型审查：不通过（含模型无返回、调用失败）的图片保持私有，其余公开。
func (s *UploadService) SetPermission(ctx context.Context, principal *auth.Principal, ids []string, permission string) (*PermissionResult, error) {
	if permission != store.PermissionPublic && permission != store.PermissionPrivate {
		return nil, fmt.Errorf("%w: unknown permission %q", ErrInvalidInput, permission)
	}
	images, err := s.ownedImages(ctx, principal, ids)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return &PermissionResult{}, nil
	}

	if permission == store.PermissionPrivate || s.moderator == nil {
		all := make([]string, 0, len(images))
		for i := range images {
			all = append(all, images[i].ID)
		}
		if err := s.repo.SetImagePermission(ctx, all, permission); err != nil {
			return nil, fmt.Errorf("set permission: %w", err)
		}
		return &PermissionResult{Published: len(all)}, nil
	}

	outcome := s.moderateForPublic(ctx, images)
	if len(outcome.publish) > 0 {
		if err := s.repo.SetImagePermission(ctx, outcome.publish, store.PermissionPublic); err != nil {
			return nil, fmt.Errorf("set permission: %w", err)
		}
	}
	if len(outcome.blocked) > 0 {
		if err := s.repo.SetImagePermission(ctx, outcome.blocked, store.PermissionPrivate); err != nil {
			return nil, fmt.Errorf("set permission: %w", err)
		}
	}
	return &PermissionResult{Published: len(outcome.publish), Blocked: outcome.blocked, Reasons: outcome.reasons}, nil
}

// moderationOutcome 汇总一次批量公开审查的结果。
type moderationOutcome struct {
	publish []string
	blocked []string
	reasons map[string]string
}

// moderateForPublic 对一批待公开图片并发执行 AI 审查，返回可公开与被拦下的 id。
// 已经是公开的图片不重复审查。
func (s *UploadService) moderateForPublic(ctx context.Context, images []store.Image) moderationOutcome {
	outcome := moderationOutcome{reasons: make(map[string]string)}
	sem := make(chan struct{}, moderationConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range images {
		image := images[i]
		if image.Permission == store.PermissionPublic {
			outcome.publish = append(outcome.publish, image.ID)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			decision, err := s.reviewImage(ctx, &image)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				outcome.blocked = append(outcome.blocked, image.ID)
				outcome.reasons[image.ID] = "审查服务不可用：" + err.Error()
			case !decision.Allowed:
				outcome.blocked = append(outcome.blocked, image.ID)
				outcome.reasons[image.ID] = decision.Reason
			default:
				outcome.publish = append(outcome.publish, image.ID)
			}
		}()
	}
	wg.Wait()
	return outcome
}

// reviewImage 读取图片内容并调用审查器。读取或审查失败均返回 error，调用方按
// 「不通过」处理（保守拒绝公开）。
func (s *UploadService) reviewImage(ctx context.Context, image *store.Image) (moderation.Decision, error) {
	if s.moderationMaxBytes > 0 && image.Size > s.moderationMaxBytes {
		return moderation.Decision{Allowed: false, Reason: "图片体积超过审查上限"}, nil
	}
	backend := s.backendFor(image)
	object, err := backend.Get(ctx, image.Key)
	if err != nil {
		return moderation.Decision{}, fmt.Errorf("read object: %w", err)
	}
	defer func() { _ = object.Close() }()
	var reader io.Reader = object
	if s.moderationMaxBytes > 0 {
		reader = io.LimitReader(object, s.moderationMaxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return moderation.Decision{}, fmt.Errorf("read object: %w", err)
	}
	return s.moderator.Review(ctx, data, image.MimeType)
}

// SetAlbum 批量把图片移动到某个相册（albumID 为 nil 表示移出相册）。目标
// 相册必须存在且归 principal 所有。
func (s *UploadService) SetAlbum(ctx context.Context, principal *auth.Principal, ids []string, albumID *string) error {
	if albumID != nil {
		album, err := s.repo.GetAlbumByID(ctx, *albumID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrAlbumNotFound
			}
			return fmt.Errorf("set album: %w", err)
		}
		if !canAccess(principal, album.UserID) {
			return ErrForbidden
		}
	}
	owned, err := s.ownedImageIDs(ctx, principal, ids)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return nil
	}
	if err := s.repo.SetImageAlbum(ctx, owned, albumID); err != nil {
		return fmt.Errorf("set album: %w", err)
	}
	return nil
}

// ownedImages 校验 ids 中的每一张图片都归 principal 所有，并返回实际存在的记录。
// 请求中的未知 id 会被忽略。
func (s *UploadService) ownedImages(ctx context.Context, principal *auth.Principal, ids []string) ([]store.Image, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: no image ids provided", ErrInvalidInput)
	}
	if len(ids) > maxBatchImages {
		return nil, fmt.Errorf("%w: at most %d images may be updated at once", ErrInvalidInput, maxBatchImages)
	}
	images, err := s.repo.ListImagesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("update images: %w", err)
	}
	for i := range images {
		if !canAccess(principal, images[i].UserID) {
			return nil, ErrForbidden
		}
	}
	return images, nil
}

// ownedImageIDs 返回归 principal 所有的图片 id。
func (s *UploadService) ownedImageIDs(ctx context.Context, principal *auth.Principal, ids []string) ([]string, error) {
	images, err := s.ownedImages(ctx, principal, ids)
	if err != nil {
		return nil, err
	}
	owned := make([]string, 0, len(images))
	for i := range images {
		owned = append(owned, images[i].ID)
	}
	return owned, nil
}

// maxBatchImages 限制单次批量操作涉及的图片数量。
const maxBatchImages = 200

// moderationConcurrency 限制批量审查时的并发请求数，避免对视觉接口造成冲击。
const moderationConcurrency = 4

// purgeConcurrency 限制级联删除对象时的并发数。
const purgeConcurrency = 8

// Delete 移除一张图片对象及其元数据，强制校验所有权并释放所有者的配额。
func (s *UploadService) Delete(ctx context.Context, principal *auth.Principal, id string) error {
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return ErrForbidden
	}
	return s.deleteImage(ctx, image)
}

// DeleteBatch 批量删除图片。它会先校验全部图片都归 principal 所有，再逐张删除，
// 返回成功与失败的数量。未知 id 会被忽略。
func (s *UploadService) DeleteBatch(ctx context.Context, principal *auth.Principal, ids []string) (deleted, failed int, err error) {
	images, err := s.ownedImages(ctx, principal, ids)
	if err != nil {
		return 0, 0, err
	}
	for i := range images {
		if delErr := s.deleteImage(ctx, &images[i]); delErr != nil {
			failed++
			continue
		}
		deleted++
	}
	return deleted, failed, nil
}

// deleteImage 删除一条已通过所有权校验的图片记录及其对象，并清理相关分享与配额。
// 对象删除为尽力而为：先删元数据，避免记录指向缺失对象。
func (s *UploadService) deleteImage(ctx context.Context, image *store.Image) error {
	if err := s.repo.Delete(ctx, image.ID); err != nil {
		return fmt.Errorf("delete image record: %w", err)
	}
	if backend := s.backendFor(image); backend != nil {
		_ = backend.Delete(context.WithoutCancel(ctx), image.Key)
	}
	_ = s.repo.DeleteSharesForTarget(context.WithoutCancel(ctx), store.ShareTargetImage, image.ID)
	if image.UserID != nil {
		if err := s.repo.ReleaseQuota(context.WithoutCancel(ctx), *image.UserID, image.Size); err != nil {
			return fmt.Errorf("delete image: release quota: %w", err)
		}
	}
	return nil
}

// PurgeOwner 删除某个用户拥有的全部图片对象与元数据，以及其相册、分享与 API
// 令牌记录。它用于管理员删除客户时的级联清理；对象删除为尽力而为。返回已处理的
// 图片数量。
func (s *UploadService) PurgeOwner(ctx context.Context, ownerID string) (int, error) {
	refs, err := s.repo.ObjectRefsByUser(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	s.purgeObjects(ctx, refs)
	if _, err := s.repo.DeleteImagesByUser(ctx, ownerID); err != nil {
		return 0, err
	}
	if _, err := s.repo.DeleteAlbumsByUser(ctx, ownerID); err != nil {
		return 0, err
	}
	if _, err := s.repo.DeleteSharesByUser(ctx, ownerID); err != nil {
		return 0, err
	}
	if _, err := s.repo.DeleteTokensByUser(ctx, ownerID); err != nil {
		return 0, err
	}
	return len(refs), nil
}

// purgeObjects 以有界并发删除一批存储对象。失败仅忽略，以便元数据清理继续进行。
func (s *UploadService) purgeObjects(ctx context.Context, refs []store.ImageObjectRef) {
	if len(refs) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	sem := make(chan struct{}, purgeConcurrency)
	var wg sync.WaitGroup
	for i := range refs {
		ref := refs[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			backend := s.manager.Resolve(storageIDValue(ref.StorageID))
			if backend == nil {
				return
			}
			_ = backend.Delete(ctx, ref.Key)
		}()
	}
	wg.Wait()
}

// reserveQuota 为 ownerID 预留 amount 字节。它返回一个释放函数（对访客与
// 管理员为空操作）以及预留是否成功。管理员不受配额限制。
func (s *UploadService) reserveQuota(ctx context.Context, principal *auth.Principal, ownerID *string, amount int64) (func(), bool, error) {
	if ownerID == nil || principal.IsAdmin() {
		return func() {}, true, nil
	}
	ok, err := s.repo.ReserveQuota(ctx, *ownerID, amount)
	if err != nil {
		return nil, false, fmt.Errorf("reserve quota: %w", err)
	}
	if !ok {
		return func() {}, false, nil
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		// 请求上下文可能已被取消；释放仍须执行，以免配额泄漏。
		_ = s.repo.ReleaseQuota(context.WithoutCancel(ctx), *ownerID, amount)
	}, true, nil
}

// ownerIDOf 返回 principal 所代表的用户 id。匿名请求（无用户 id）返回 nil；
// 已配置的访客主体带有内置 Guest 账户 id，因此访客上传会计入其配额。
func ownerIDOf(principal *auth.Principal) *string {
	if principal == nil || principal.UserID == "" {
		return nil
	}
	id := principal.UserID
	return &id
}

// canAccess 报告 principal 是否可以访问由 ownerID 拥有的资源。
func canAccess(principal *auth.Principal, ownerID *string) bool {
	if principal.IsAdmin() {
		return true
	}
	return sameOwner(principal, ownerID)
}

// sameOwner 报告 principal 是否拥有 ownerID 对应的资源。
func sameOwner(principal *auth.Principal, ownerID *string) bool {
	current := ownerIDOf(principal)
	switch {
	case current == nil && ownerID == nil:
		return true
	case current == nil || ownerID == nil:
		return false
	default:
		return *current == *ownerID
	}
}

func toDTO(image *store.Image, mediaBaseURL string) *ImageDTO {
	permission := image.Permission
	if permission == "" {
		permission = store.PermissionPrivate
	}
	thumbnail := mediaThumbnailURL(mediaBaseURL, image.Key)
	if thumbnail == "" {
		thumbnail = image.URL
	}
	return &ImageDTO{
		ID:           image.ID,
		Key:          image.Key,
		StorageID:    storageIDValue(image.StorageID),
		AlbumID:      storageIDValue(image.AlbumID),
		Permission:   permission,
		OriginalName: image.OriginalName,
		Filename:     image.Filename,
		Hash:         image.Hash,
		URL:          image.URL,
		Thumbnail:    thumbnail,
		Size:         image.Size,
		MimeType:     image.MimeType,
		Width:        image.Width,
		Height:       image.Height,
		CreatedAt:    image.CreatedAt,
	}
}

// thumbnailWidth 与 thumbnailQuality 是列表缩略图的默认参数。缩略图始终经
// 由本实例的 /i/* 端点生成，因此与存储后端无关。
const (
	thumbnailWidth   = 480
	thumbnailQuality = 75
)

// mediaThumbnailURL 构造一张图片的缩略图 URL。baseURL 为空或无 key 时返回
// 空字符串，由调用方回退到原图 URL。
func mediaThumbnailURL(baseURL, key string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" || key == "" {
		return ""
	}
	return base + "/i/" + key + "?w=" + strconv.Itoa(thumbnailWidth) +
		"&q=" + strconv.Itoa(thumbnailQuality)
}

// storageIDPtr 将非空字符串转换为指针；空字符串返回 nil。
func storageIDPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// storageIDValue 安全地取出存储 id 指针的值；nil 返回空字符串。
func storageIDValue(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

// stringValue 安全地取出字符串指针的值；nil 返回空字符串。
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// randomKey 构建一个按日期分区、随机且不可猜测的存储键，形如
// `2006/01/02/<uuid><扩展名>`。存储键不再与内容哈希相关，因此私有图片无法
// 通过已知内容推断出 URL；内容哈希单独记录在数据库中用于按所有者去重。
func randomKey(mimeType string) string {
	return path.Join(time.Now().UTC().Format("2006/01/02"), uuid.NewString()+extensionForMIME(mimeType))
}

// contentHash 返回数据的 sha256 十六进制摘要。
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// maxOriginalNameBytes 限制原始文件名的字节长度，与数据库列宽保持一致。
const maxOriginalNameBytes = 255

// sanitizeOriginalName 规范化上传时的原始文件名：仅保留基础名、去除路径分隔与
// 控制字符，并限制长度，避免回显危险内容。截断按 UTF-8 边界进行，避免截断
// 多字节字符。
func sanitizeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// 去掉任何目录部分（同时兼容 Windows 反斜杠）。
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == '/' || r < 0x20 {
			return -1
		}
		return r
	}, name)
	if len(name) > maxOriginalNameBytes {
		name = truncateUTF8(name, maxOriginalNameBytes)
	}
	return name
}

// truncateUTF8 将 s 截断到不超过 maxBytes 字节，且不切断 UTF-8 字符。
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// decodeDimensions 返回一张已编码图片的像素尺寸，当格式无法解码时返回零值。
func decodeDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// probeObject 读取某个已存储对象的一段有界前缀，并报告其检测到的媒体类型
// 和像素尺寸。当对象无法读取时，它返回空媒体类型和零尺寸。
func (s *UploadService) probeObject(ctx context.Context, backend storage.Storage, key string) (string, int, int) {
	object, err := backend.Get(ctx, key)
	if err != nil {
		return "", 0, 0
	}
	defer func() {
		_ = object.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(object, dimensionProbeLimit))
	if err != nil || len(data) == 0 {
		return "", 0, 0
	}
	width, height := decodeDimensions(data)
	return http.DetectContentType(data), width, height
}
