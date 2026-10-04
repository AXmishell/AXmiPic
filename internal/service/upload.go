// Package service 包含 AXmiPic 的业务逻辑。
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
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

// ErrForbidden 在调用者无权访问某个资源时返回。
var ErrForbidden = errors.New("service: forbidden")

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
	// OriginalName 为上传时的原始文件名；Filename 为重命名后的存储文件名；
	// Hash 为内容 sha256 十六进制摘要。
	OriginalName string    `json:"original_name"`
	Filename     string    `json:"filename"`
	Hash         string    `json:"hash"`
	URL          string    `json:"url"`
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	CreatedAt    time.Time `json:"created_at"`
}

// ListResult 是图片的分页集合。
type ListResult struct {
	Items    []ImageDTO `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// UploadService 协调校验、去重、存储和元数据。
type UploadService struct {
	repo    *store.Repository
	manager *storage.Manager
	policy  UploadPolicy
	allowed map[string]struct{}
}

// NewUploadService 构造一个 UploadService。
func NewUploadService(repo *store.Repository, manager *storage.Manager, policy UploadPolicy) *UploadService {
	allowed := make(map[string]struct{}, len(policy.AllowedMIMETypes))
	for _, mimeType := range policy.AllowedMIMETypes {
		allowed[mimeType] = struct{}{}
	}
	return &UploadService{repo: repo, manager: manager, policy: policy, allowed: allowed}
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

// Upload 校验、存储并记录一张归 principal 所有的图片。相同内容会按其
// 内容寻址键进行去重。
func (s *UploadService) Upload(ctx context.Context, principal *auth.Principal, in UploadInput) (*ImageDTO, error) {
	size := int64(len(in.Data))
	if size > s.policy.MaxSizeBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, size, s.policy.MaxSizeBytes)
	}
	if _, ok := s.allowed[in.MimeType]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, in.MimeType)
	}

	// 计算内容哈希，并据此生成「重命名」后的存储文件名与键。
	hash := contentHash(in.Data)
	key := hashKey(hash, in.MimeType)
	filename := path.Base(key)
	originalName := sanitizeOriginalName(in.OriginalName)

	existing, err := s.repo.GetByKey(ctx, key)
	if err == nil {
		return toDTO(existing), nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("upload: lookup existing image: %w", err)
	}

	ownerID := ownerIDOf(principal)
	release, ok, err := s.reserveQuota(ctx, principal, ownerID, size)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrQuotaExceeded
	}
	committed := false
	defer func() {
		if !committed {
			release()
		}
	}()

	// 如果上一次写入被中断，某个内容寻址对象可能已存在却没有元数据行；
	// 此时复用它，而不是再次写入。
	backend := s.manager.Current()
	if backend == nil {
		return nil, fmt.Errorf("%w: no storage backend configured", ErrStorageConfig)
	}
	currentID := s.manager.CurrentID()
	exists, err := backend.Exists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("upload: check object existence: %w", err)
	}
	if !exists {
		if err := backend.Put(ctx, key, bytes.NewReader(in.Data), size, in.MimeType); err != nil {
			return nil, fmt.Errorf("upload: store object: %w", err)
		}
	}

	width, height := decodeDimensions(in.Data)
	image := &store.Image{
		ID:           uuid.NewString(),
		Key:          key,
		UserID:       ownerID,
		StorageID:    storageIDPtr(currentID),
		OriginalName: originalName,
		Filename:     filename,
		Hash:         hash,
		URL:          s.urlFor(backend, key),
		Size:         size,
		MimeType:     in.MimeType,
		Width:        width,
		Height:       height,
	}
	if err := s.repo.Create(ctx, image); err != nil {
		// 并发上传相同内容可能已先插入该行；复用那条记录并释放我们的预留。
		if concurrent, getErr := s.repo.GetByKey(ctx, key); getErr == nil {
			return toDTO(concurrent), nil
		}
		return nil, fmt.Errorf("upload: record image: %w", err)
	}
	committed = true
	return toDTO(image), nil
}

// Presign 校验直传请求，从存储后端签发一个预签名请求，并记录一条待确认的
// 上传供后续确认。
func (s *UploadService) Presign(ctx context.Context, principal *auth.Principal, in PresignInput) (*PresignResult, error) {
	presigner := s.manager.PresignerFor("")
	if presigner == nil {
		return nil, ErrPresignUnsupported
	}
	if _, ok := s.allowed[in.MimeType]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, in.MimeType)
	}
	if in.Size <= 0 {
		return nil, fmt.Errorf("%w: size must be positive", ErrInvalidInput)
	}
	if in.Size > s.policy.MaxSizeBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, in.Size, s.policy.MaxSizeBytes)
	}

	backend := s.manager.Current()
	key := directKey(in.MimeType)
	req, err := presigner.PresignPut(ctx, key, storage.PresignOptions{
		ContentType:         in.MimeType,
		AllowedContentTypes: s.policy.AllowedMIMETypes,
		MaxSize:             s.policy.MaxSizeBytes,
		Expires:             s.policy.PresignExpiry,
	})
	if err != nil {
		return nil, fmt.Errorf("presign: %w", err)
	}

	pending := &store.PendingUpload{
		Key:       key,
		UserID:    ownerIDOf(principal),
		StorageID: storageIDPtr(s.manager.CurrentID()),
		MimeType:  in.MimeType,
		MaxSize:   s.policy.MaxSizeBytes,
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
		return toDTO(existing), nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("confirm: lookup existing image: %w", err)
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
	if info.Size > s.policy.MaxSizeBytes {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: object is %d bytes", ErrFileTooLarge, info.Size)
	}
	if _, ok := s.allowed[info.ContentType]; !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, info.ContentType)
	}
	// 验证对象实际上看起来像一张被允许的图片，而不是仅凭客户端声明的
	// 内容类型来信任。
	detected, width, height := s.probeObject(ctx, backend, key)
	if _, ok := s.allowed[detected]; !ok {
		_ = backend.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: object content is %q", ErrUnsupportedType, detected)
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
	committed := false
	defer func() {
		if !committed {
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
			return toDTO(concurrent), nil
		}
		return nil, fmt.Errorf("confirm: record image: %w", err)
	}
	_ = s.repo.DeletePendingUpload(ctx, key)
	committed = true
	return toDTO(image), nil
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
// 自己拥有的图片。
func (s *UploadService) List(ctx context.Context, principal *auth.Principal, page, pageSize int) (*ListResult, error) {
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

	filter := ""
	if !principal.IsAdmin() {
		if principal.IsGuest() {
			return &ListResult{Items: []ImageDTO{}, Total: 0, Page: page, PageSize: pageSize}, nil
		}
		filter = principal.UserID
	}

	images, total, err := s.repo.List(ctx, filter, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	items := make([]ImageDTO, 0, len(images))
	for i := range images {
		items = append(items, *toDTO(&images[i]))
	}
	return &ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
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
	return toDTO(image), nil
}

// GetByKey 按其存储键返回单张图片。公开服务不强制校验所有权。
func (s *UploadService) GetByKey(ctx context.Context, key string) (*ImageDTO, error) {
	image, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get image by key: %w", err)
	}
	return toDTO(image), nil
}

// Delete 移除一张图片对象及其元数据，强制校验所有权并释放所有者的配额。
func (s *UploadService) Delete(ctx context.Context, principal *auth.Principal, id string) error {
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return ErrForbidden
	}
	backend := s.backendFor(image)
	if err := backend.Delete(ctx, image.Key); err != nil {
		return fmt.Errorf("delete image object: %w", err)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete image record: %w", err)
	}
	if image.UserID != nil {
		if err := s.repo.ReleaseQuota(ctx, *image.UserID, image.Size); err != nil {
			return fmt.Errorf("delete image: release quota: %w", err)
		}
	}
	return nil
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

// ownerIDOf 返回 principal 所代表的用户 id，访客则返回 nil。
func ownerIDOf(principal *auth.Principal) *string {
	if principal.IsGuest() || principal.UserID == "" {
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

func toDTO(image *store.Image) *ImageDTO {
	return &ImageDTO{
		ID:           image.ID,
		Key:          image.Key,
		StorageID:    storageIDValue(image.StorageID),
		OriginalName: image.OriginalName,
		Filename:     image.Filename,
		Hash:         image.Hash,
		URL:          image.URL,
		Size:         image.Size,
		MimeType:     image.MimeType,
		Width:        image.Width,
		Height:       image.Height,
		CreatedAt:    image.CreatedAt,
	}
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

// directKey 为直传到存储的上传构建一个按日期分区、唯一的键，此类上传
// 服务器不会看到其字节内容，因而无法进行内容寻址。
func directKey(mimeType string) string {
	return path.Join(time.Now().UTC().Format("2006/01/02"), uuid.NewString()+extensionForMIME(mimeType))
}

// contentHash 返回数据的 sha256 十六进制摘要。
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hashKey 依据内容哈希构建「重命名」后的、以斜杠分隔的存储键：
// 前两级为哈希前缀的目录，最后一段为 `<hash><扩展名>`。
func hashKey(hash, mimeType string) string {
	return path.Join(hash[0:2], hash[2:4], hash+extensionForMIME(mimeType))
}

// sanitizeOriginalName 规范化上传时的原始文件名：仅保留基础名、去除路径分隔与
// 控制字符，并限制长度，避免回显危险内容。
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
	if len(name) > 255 {
		name = name[:255]
	}
	return name
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
