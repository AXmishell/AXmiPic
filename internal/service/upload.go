// Package service contains the business logic of AXmiPic.
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

// ErrFileTooLarge is returned when an upload exceeds the configured size limit.
var ErrFileTooLarge = errors.New("service: file exceeds the maximum allowed size")

// ErrUnsupportedType is returned when an upload's media type is not allowed.
var ErrUnsupportedType = errors.New("service: unsupported media type")

// ErrPresignUnsupported is returned when the active storage backend cannot
// issue presigned direct-upload requests.
var ErrPresignUnsupported = errors.New("service: presigned upload is not supported by the configured storage driver")

// ErrInvalidInput is returned when a request is malformed.
var ErrInvalidInput = errors.New("service: invalid input")

// ErrQuotaExceeded is returned when an upload would exceed the owner's quota.
var ErrQuotaExceeded = errors.New("service: storage quota exceeded")

// ErrForbidden is returned when a caller may not access a resource.
var ErrForbidden = errors.New("service: forbidden")

// ErrNotFound is returned when a requested image does not exist.
var ErrNotFound = store.ErrNotFound

// dimensionProbeLimit bounds how much of an object is read to decode image
// dimensions, avoiding a full download during confirm.
const dimensionProbeLimit = 1 << 20

// mimeExtensions maps an accepted MIME type to the key extension used on disk.
var mimeExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// UploadPolicy constrains what UploadService accepts.
type UploadPolicy struct {
	MaxSizeBytes     int64
	AllowedMIMETypes []string
	PresignExpiry    time.Duration
}

// UploadInput is an upload request ready to be persisted.
type UploadInput struct {
	Data     []byte
	MimeType string
}

// PresignInput requests a presigned direct-upload for a piece of content.
type PresignInput struct {
	MimeType string
	Size     int64
}

// PresignResult tells a client how to upload an object directly to storage.
type PresignResult struct {
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ImageDTO is the API representation of a stored image.
type ImageDTO struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	CreatedAt time.Time `json:"created_at"`
}

// ListResult is a paginated collection of images.
type ListResult struct {
	Items    []ImageDTO `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// UploadService coordinates validation, deduplication, storage, and metadata.
type UploadService struct {
	repo    *store.Repository
	storage storage.Storage
	policy  UploadPolicy
	allowed map[string]struct{}
}

// NewUploadService constructs an UploadService.
func NewUploadService(repo *store.Repository, backend storage.Storage, policy UploadPolicy) *UploadService {
	allowed := make(map[string]struct{}, len(policy.AllowedMIMETypes))
	for _, mimeType := range policy.AllowedMIMETypes {
		allowed[mimeType] = struct{}{}
	}
	return &UploadService{repo: repo, storage: backend, policy: policy, allowed: allowed}
}

// Upload validates, stores, and records an image owned by principal. Identical
// content is deduplicated by its content-addressed key.
func (s *UploadService) Upload(ctx context.Context, principal *auth.Principal, in UploadInput) (*ImageDTO, error) {
	size := int64(len(in.Data))
	if size > s.policy.MaxSizeBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d bytes", ErrFileTooLarge, size, s.policy.MaxSizeBytes)
	}
	if _, ok := s.allowed[in.MimeType]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, in.MimeType)
	}

	key := contentKey(in.Data, in.MimeType)

	existing, err := s.repo.GetByKey(ctx, key)
	if err == nil {
		return toDTO(existing), nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("upload: lookup existing image: %w", err)
	}

	ownerID := ownerIDOf(principal)
	release, ok, err := s.reserveQuota(ctx, ownerID, size)
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

	// A content-addressed object may already exist without a metadata row if a
	// previous write was interrupted; reuse it instead of writing again.
	exists, err := s.storage.Exists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("upload: check object existence: %w", err)
	}
	if !exists {
		if err := s.storage.Put(ctx, key, bytes.NewReader(in.Data), size, in.MimeType); err != nil {
			return nil, fmt.Errorf("upload: store object: %w", err)
		}
	}

	width, height := decodeDimensions(in.Data)
	image := &store.Image{
		ID:       uuid.NewString(),
		Key:      key,
		UserID:   ownerID,
		URL:      s.storage.URL(key),
		Size:     size,
		MimeType: in.MimeType,
		Width:    width,
		Height:   height,
	}
	if err := s.repo.Create(ctx, image); err != nil {
		// A concurrent upload of identical content may have inserted the row
		// first; reuse that record and release our reservation.
		if concurrent, getErr := s.repo.GetByKey(ctx, key); getErr == nil {
			return toDTO(concurrent), nil
		}
		return nil, fmt.Errorf("upload: record image: %w", err)
	}
	committed = true
	return toDTO(image), nil
}

// Presign validates a direct-upload request, issues a presigned request from
// the storage backend, and records a pending upload for later confirmation.
func (s *UploadService) Presign(ctx context.Context, principal *auth.Principal, in PresignInput) (*PresignResult, error) {
	presigner, ok := s.storage.(storage.Presigner)
	if !ok {
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
		MimeType:  in.MimeType,
		MaxSize:   s.policy.MaxSizeBytes,
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.repo.CreatePendingUpload(ctx, pending); err != nil {
		return nil, fmt.Errorf("presign: record pending upload: %w", err)
	}

	return &PresignResult{
		Key:       key,
		URL:       s.storage.URL(key),
		UploadURL: req.URL,
		Method:    req.Method,
		Fields:    req.Fields,
		Headers:   req.Headers,
		ExpiresAt: req.ExpiresAt,
	}, nil
}

// Confirm verifies that a presigned object was uploaded and records its
// metadata. It is idempotent for an already-recorded key.
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

	info, err := s.storage.Stat(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("confirm: object was not uploaded: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("confirm: stat object: %w", err)
	}
	if info.Size > s.policy.MaxSizeBytes {
		_ = s.storage.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: object is %d bytes", ErrFileTooLarge, info.Size)
	}
	if _, ok := s.allowed[info.ContentType]; !ok {
		_ = s.storage.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, info.ContentType)
	}

	ownerID := pending.UserID
	release, ok, err := s.reserveQuota(ctx, ownerID, info.Size)
	if err != nil {
		return nil, err
	}
	if !ok {
		_ = s.storage.Delete(ctx, key)
		_ = s.repo.DeletePendingUpload(ctx, key)
		return nil, ErrQuotaExceeded
	}
	committed := false
	defer func() {
		if !committed {
			release()
		}
	}()

	width, height := s.probeDimensions(ctx, key)
	image := &store.Image{
		ID:       uuid.NewString(),
		Key:      key,
		UserID:   ownerID,
		URL:      s.storage.URL(key),
		Size:     info.Size,
		MimeType: info.ContentType,
		Width:    width,
		Height:   height,
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

// CleanupExpired removes pending uploads that have expired as of now and
// deletes their unconfirmed storage objects. It returns the number of storage
// objects removed.
func (s *UploadService) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	pending, err := s.repo.ExpiredPendingUploads(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("cleanup: list expired pending uploads: %w", err)
	}
	removed := 0
	for i := range pending {
		key := pending[i].Key
		if _, err := s.repo.GetByKey(ctx, key); err == nil {
			// The upload was confirmed after all; only drop the stale row.
			if delErr := s.repo.DeletePendingUpload(ctx, key); delErr != nil {
				continue
			}
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err := s.storage.Delete(ctx, key); err != nil {
			continue
		}
		if err := s.repo.DeletePendingUpload(ctx, key); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}

// List returns a page of images visible to principal: all images for admins,
// only owned images otherwise.
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

// Get returns a single image by id, enforcing ownership.
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

// GetByKey returns a single image by its storage key. Public serving does not
// enforce ownership.
func (s *UploadService) GetByKey(ctx context.Context, key string) (*ImageDTO, error) {
	image, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get image by key: %w", err)
	}
	return toDTO(image), nil
}

// Delete removes an image object and its metadata, enforcing ownership and
// releasing the owner's quota.
func (s *UploadService) Delete(ctx context.Context, principal *auth.Principal, id string) error {
	image, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete image: %w", err)
	}
	if !canAccess(principal, image.UserID) {
		return ErrForbidden
	}
	if err := s.storage.Delete(ctx, image.Key); err != nil {
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

// reserveQuota reserves amount bytes for ownerID. It returns a release function
// (a no-op for guests) and whether the reservation succeeded.
func (s *UploadService) reserveQuota(ctx context.Context, ownerID *string, amount int64) (func(), bool, error) {
	if ownerID == nil {
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
		// The request context may already be canceled; release must still run so
		// quota is not leaked.
		_ = s.repo.ReleaseQuota(context.WithoutCancel(ctx), *ownerID, amount)
	}, true, nil
}

// ownerIDOf returns the user id a principal acts as, or nil for guests.
func ownerIDOf(principal *auth.Principal) *string {
	if principal.IsGuest() || principal.UserID == "" {
		return nil
	}
	id := principal.UserID
	return &id
}

// canAccess reports whether principal may access a resource owned by ownerID.
func canAccess(principal *auth.Principal, ownerID *string) bool {
	if principal.IsAdmin() {
		return true
	}
	return sameOwner(principal, ownerID)
}

// sameOwner reports whether principal owns a resource with ownerID.
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
		ID:        image.ID,
		Key:       image.Key,
		URL:       image.URL,
		Size:      image.Size,
		MimeType:  image.MimeType,
		Width:     image.Width,
		Height:    image.Height,
		CreatedAt: image.CreatedAt,
	}
}

// directKey builds a date-partitioned, unique key for a direct-to-storage
// upload, where the server never sees the bytes and cannot content-address.
func directKey(mimeType string) string {
	return path.Join(time.Now().UTC().Format("2006/01/02"), uuid.NewString()+mimeExtensions[mimeType])
}

// contentKey builds a content-addressed, slash-separated storage key.
func contentKey(data []byte, mimeType string) string {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	return path.Join(hash[0:2], hash[2:4], hash+mimeExtensions[mimeType])
}

// decodeDimensions returns the pixel dimensions of an encoded image, or zeros
// when the format cannot be decoded.
func decodeDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// probeDimensions reads a bounded prefix of a stored object to decode its
// dimensions, returning zeros on any failure.
func (s *UploadService) probeDimensions(ctx context.Context, key string) (int, int) {
	object, err := s.storage.Get(ctx, key)
	if err != nil {
		return 0, 0
	}
	defer func() {
		_ = object.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(object, dimensionProbeLimit))
	if err != nil {
		return 0, 0
	}
	return decodeDimensions(data)
}
