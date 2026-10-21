package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/store"
)

var (
	// ErrXImportUnavailable 在 X 导入插件不可用时返回。
	ErrXImportUnavailable = errors.New("service: X import is unavailable")
	// ErrXImportFailed 在解析接口或下载失败时返回，通常对应上游错误。
	ErrXImportFailed = errors.New("service: X import failed")
)

const (
	xImportMaxDownload     = 32 << 20
	xImportTimeout         = 45 * time.Second
	xImportDownloadTimeout = 20 * time.Second
)

// XImportInput 是导入一条 X 帖子的请求。
type XImportInput struct {
	// URL 是用户粘贴的 X/Twitter 状态链接。
	URL string
	// MediaIDs 为空时导入全部图片；否则只导入选中的图片。
	MediaIDs []string
	// Permission 为导入后的可见性：public（默认）或 private。
	Permission string
}

// XImportItem 是单张图片的导入结果。
type XImportItem struct {
	Image      *ImageDTO `json:"image,omitempty"`
	Status     string    `json:"status"`
	Permission string    `json:"permission,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// XImportResult 是一次导入的汇总。
type XImportResult struct {
	Post     *XResolvedPost `json:"post"`
	Items    []XImportItem  `json:"items"`
	Imported int            `json:"imported"`
	Existing int            `json:"existing"`
	Blocked  int            `json:"blocked"`
	Failed   int            `json:"failed"`
}

// XImportService 负责 X 链接的解析、图片下载与入库。解析交给插件，下载与
// 上传仍由宿主完成，从而完整复用配额、去重、安全扫描与内容审核。
type XImportService struct {
	upload  *UploadService
	plugins *PluginService
	logger  *slog.Logger
}

// NewXImportService 构造 X 导入服务。
func NewXImportService(upload *UploadService, plugins *PluginService, logger *slog.Logger) *XImportService {
	if logger == nil {
		logger = slog.Default()
	}
	return &XImportService{
		upload:  upload,
		plugins: plugins,
		logger:  logger,
	}
}

// clientForProxy 构造下载客户端；proxy 为 nil 时沿用默认环境变量代理。
func (s *XImportService) clientForProxy(proxy *url.URL) *http.Client {
	return &http.Client{
		Timeout:   xImportTimeout,
		Transport: transportWithProxy(proxy),
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if !validXMediaHost(req.URL.Hostname()) {
				return fmt.Errorf("x import: redirect to unsupported host %q", req.URL.Hostname())
			}
			return nil
		},
	}
}

// Preview 解析并返回帖子信息，不下载图片。
func (s *XImportService) Preview(ctx context.Context, rawURL string) (*XResolvedPost, error) {
	return s.resolve(ctx, rawURL)
}

// Import 解析链接、下载选中图片、上传并以指定可见性入库。默认公开到广场。
func (s *XImportService) Import(ctx context.Context, principal *auth.Principal, in XImportInput) (*XImportResult, error) {
	if principal == nil || principal.IsGuest() {
		return nil, fmt.Errorf("%w: login is required to import from X", ErrForbidden)
	}
	permission := strings.ToLower(strings.TrimSpace(in.Permission))
	if permission == "" {
		permission = store.PermissionPublic
	}
	if permission != store.PermissionPublic && permission != store.PermissionPrivate {
		return nil, fmt.Errorf("%w: permission must be public or private", ErrInvalidInput)
	}

	post, err := s.resolve(ctx, in.URL)
	if err != nil {
		return nil, err
	}
	media, err := selectXMedia(post.Media, in.MediaIDs)
	if err != nil {
		return nil, err
	}

	policy, _, err := s.upload.policyFor(ctx, principal)
	if err != nil {
		return nil, err
	}
	maxDownload := policy.MaxSizeBytes
	if maxDownload <= 0 || maxDownload > xImportMaxDownload {
		maxDownload = xImportMaxDownload
	}
	proxy, err := s.plugins.XResolverProxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrXImportFailed, err)
	}
	client := s.clientForProxy(proxy)

	meta := ImageMeta{
		AuthorName:     post.Author.Name,
		AuthorHandle:   post.Author.Handle,
		AuthorPlatform: "x",
		SourceURL:      post.SourceURL,
		SourcePostID:   post.PostID,
	}
	normalizedMeta, err := NormalizeImageMeta(meta)
	if err != nil {
		return nil, err
	}

	result := &XImportResult{Post: post}
	ownerKey := stringValue(ownerIDOf(principal))
	for i, item := range media {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		downloadCtx, cancel := context.WithTimeout(ctx, xImportDownloadTimeout)
		data, mimeType, err := s.download(downloadCtx, client, item.URL, maxDownload)
		cancel()
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, XImportItem{Status: "failed", Error: err.Error()})
			continue
		}
		hash := contentHash(data)
		existing, lookupErr := s.upload.repo.GetByHashAndUser(ctx, hash, ownerKey)
		existed := lookupErr == nil
		if lookupErr != nil && !errors.Is(lookupErr, store.ErrNotFound) {
			result.Failed++
			result.Items = append(result.Items, XImportItem{Status: "failed", Error: "lookup existing image failed"})
			continue
		}

		originalName := fmt.Sprintf("x-%s-%d%s", post.PostID, i+1, extensionForMIME(mimeType))
		dto, err := s.upload.Upload(ctx, principal, UploadInput{
			Data:         data,
			MimeType:     mimeType,
			OriginalName: originalName,
			Meta:         normalizedMeta,
		})
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, XImportItem{Status: "failed", Error: err.Error()})
			continue
		}

		// 已有图片如果还没有作者/来源信息，系统可补写一次署名；不覆盖用户或
		// 管理员已经填写过的内容。
		if existed && existing != nil && emptyAttribution(*existing) {
			backfilled, backfillErr := s.upload.repo.UpdateImage(ctx, existing.ID, store.ImagePatch{
				AuthorName:     &normalizedMeta.AuthorName,
				AuthorHandle:   &normalizedMeta.AuthorHandle,
				AuthorPlatform: &normalizedMeta.AuthorPlatform,
				SourceURL:      &normalizedMeta.SourceURL,
				SourcePostID:   &normalizedMeta.SourcePostID,
			})
			if backfillErr == nil {
				dto = toDTO(backfilled, s.upload.mediaBaseURL)
			}
		}

		status := "imported"
		if existed {
			status = "existing"
		}
		if permission == store.PermissionPublic {
			permissionResult, permErr := s.upload.SetPermission(ctx, principal, []string{dto.ID}, store.PermissionPublic)
			if permErr != nil {
				result.Failed++
				result.Items = append(result.Items, XImportItem{Image: dto, Status: "failed", Error: permErr.Error()})
				continue
			}
			if containsString(permissionResult.Blocked, dto.ID) {
				status = "blocked"
				result.Blocked++
			} else {
				dto.Permission = store.PermissionPublic
			}
		}

		if status == "imported" {
			result.Imported++
		} else if status == "existing" {
			result.Existing++
		}
		result.Items = append(result.Items, XImportItem{
			Image:      dto,
			Status:     status,
			Permission: dto.Permission,
		})
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("%w: no images were imported", ErrInvalidInput)
	}
	return result, nil
}

// resolve 调用当前的内置 X 解析插件。
func (s *XImportService) resolve(ctx context.Context, rawURL string) (*XResolvedPost, error) {
	if s.plugins == nil {
		return nil, ErrXImportUnavailable
	}
	ref, err := ParseXStatusURL(rawURL)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{"url": ref.CanonicalURL})
	if err != nil {
		return nil, err
	}
	out, err := s.plugins.Invoke(ctx, plugin.CategoryImportX, BuiltinXResolver, "resolve", payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrXImportFailed, err)
	}
	var post XResolvedPost
	if err := json.Unmarshal(out, &post); err != nil {
		return nil, fmt.Errorf("x import: invalid resolver response: %w", err)
	}
	return normalizeResolvedPost(&post, ref)
}

// normalizeResolvedPost 校验解析结果，过滤非图片与不受信任的媒体地址。
func normalizeResolvedPost(post *XResolvedPost, ref XStatusRef) (*XResolvedPost, error) {
	if post == nil {
		return nil, fmt.Errorf("x import: empty resolver response")
	}
	if post.Platform == "" {
		post.Platform = "x"
	}
	if !strings.EqualFold(post.Platform, "x") {
		return nil, fmt.Errorf("%w: unsupported import platform", ErrInvalidInput)
	}
	if post.PostID == "" {
		post.PostID = ref.PostID
	}
	if post.PostID != ref.PostID {
		return nil, fmt.Errorf("%w: resolver returned a different post", ErrInvalidInput)
	}
	if post.Author.Handle == "" {
		post.Author.Handle = ref.Handle
	}
	post.Author.Handle = strings.TrimPrefix(strings.TrimSpace(post.Author.Handle), "@")
	post.Author.Name = strings.TrimSpace(post.Author.Name)
	if post.Author.Handle != "" && !xHandlePattern.MatchString(post.Author.Handle) {
		return nil, fmt.Errorf("%w: invalid author handle", ErrInvalidInput)
	}
	post.SourceURL = canonicalXURL(post.Author.Handle, post.PostID)

	filtered := make([]XResolvedMedia, 0, len(post.Media))
	for _, media := range post.Media {
		if !strings.EqualFold(media.Type, "photo") {
			continue
		}
		if !validXMediaURL(media.URL) {
			continue
		}
		media.Type = "photo"
		media.URL = strings.TrimSpace(media.URL)
		filtered = append(filtered, media)
		if len(filtered) >= xResolverMaxPhotos {
			break
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("%w: the X post contains no importable photos", ErrInvalidInput)
	}
	post.Media = filtered
	if _, err := NormalizeImageMeta(ImageMeta{
		AuthorName:     post.Author.Name,
		AuthorHandle:   post.Author.Handle,
		AuthorPlatform: "x",
		SourceURL:      post.SourceURL,
		SourcePostID:   post.PostID,
	}); err != nil {
		return nil, err
	}
	return post, nil
}

// selectXMedia 按 media_ids 过滤待导入图片；未提供时全部导入。
func selectXMedia(media []XResolvedMedia, ids []string) ([]XResolvedMedia, error) {
	if len(ids) == 0 {
		return media, nil
	}
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[strings.TrimSpace(id)] = struct{}{}
	}
	out := make([]XResolvedMedia, 0, len(ids))
	for _, item := range media {
		if _, ok := selected[item.ID]; ok {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no matching media selected", ErrInvalidInput)
	}
	return out, nil
}

// download 下载一张 X 图片，并强制 HTTPS、域名白名单与体积上限。
func (s *XImportService) download(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, string, error) {
	if !validXMediaURL(rawURL) {
		return nil, "", fmt.Errorf("%w: unsupported image url", ErrInvalidInput)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "AXmiPic/X-Import")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download image: upstream returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("%w: image exceeds %d bytes", ErrFileTooLarge, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("%w: image exceeds %d bytes", ErrFileTooLarge, maxBytes)
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", fmt.Errorf("%w: downloaded content is not an image", ErrUnsupportedType)
	}
	return data, mimeType, nil
}

func validXMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	if parsed.User != nil {
		return false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "pbs.twimg.com" || strings.HasSuffix(host, ".twimg.com")
}

func validXMediaHost(host string) bool {
	host = strings.ToLower(host)
	return host == "pbs.twimg.com" || strings.HasSuffix(host, ".twimg.com")
}

func emptyAttribution(image store.Image) bool {
	return image.AuthorName == "" && image.AuthorHandle == "" && image.SourceURL == ""
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
