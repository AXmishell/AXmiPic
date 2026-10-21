package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ImageMeta 描述图片的原作者与来源信息。所有字段都可以为空。
type ImageMeta struct {
	AuthorName     string
	AuthorHandle   string
	AuthorPlatform string
	SourceURL      string
	SourcePostID   string
}

var (
	xHandlePattern   = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	xPostIDPattern   = regexp.MustCompile(`^[0-9]{1,32}$`)
	authorPlatformRE = regexp.MustCompile(`^[a-z0-9._-]{1,20}$`)
	xStatusPathRE    = regexp.MustCompile(`^/([A-Za-z0-9_]{1,15})/status/([0-9]{1,20})(?:/.*)?$`)
	xPostPathRE      = regexp.MustCompile(`^/(?:i/)?(?:web/)?status/([0-9]{1,20})(?:/.*)?$`)
)

// XStatusRef 是从 X/Twitter 状态链接中解析出的规范化引用。
type XStatusRef struct {
	// CanonicalURL 是站内使用的稳定链接，统一改写为 x.com。
	CanonicalURL string
	// Handle 是链接路径中的用户名；/i/status/ 形式为空。
	Handle string
	// PostID 是状态 id（雪花 id 的数字形式）。
	PostID string
}

// NormalizeImageMeta 规范化并校验作者/来源字段。空字段保持为空；非法内容
// 返回包装后的 ErrInvalidInput。
func NormalizeImageMeta(meta ImageMeta) (ImageMeta, error) {
	meta.AuthorName = strings.TrimSpace(meta.AuthorName)
	meta.AuthorHandle = strings.TrimPrefix(strings.TrimSpace(meta.AuthorHandle), "@")
	meta.AuthorPlatform = strings.ToLower(strings.TrimSpace(meta.AuthorPlatform))
	meta.SourceURL = strings.TrimSpace(meta.SourceURL)
	meta.SourcePostID = strings.TrimSpace(meta.SourcePostID)

	if meta.AuthorName != "" {
		if utf8.RuneCountInString(meta.AuthorName) > 100 {
			return ImageMeta{}, fmt.Errorf("%w: author name is too long", ErrInvalidInput)
		}
		for _, r := range meta.AuthorName {
			if unicode.IsControl(r) {
				return ImageMeta{}, fmt.Errorf("%w: author name contains control characters", ErrInvalidInput)
			}
		}
	}
	if meta.AuthorHandle != "" && !xHandlePattern.MatchString(meta.AuthorHandle) {
		return ImageMeta{}, fmt.Errorf("%w: invalid author handle", ErrInvalidInput)
	}
	if meta.AuthorPlatform != "" && !authorPlatformRE.MatchString(meta.AuthorPlatform) {
		return ImageMeta{}, fmt.Errorf("%w: invalid author platform", ErrInvalidInput)
	}
	if meta.SourcePostID != "" && !xPostIDPattern.MatchString(meta.SourcePostID) {
		return ImageMeta{}, fmt.Errorf("%w: invalid source post id", ErrInvalidInput)
	}
	if meta.AuthorHandle != "" && meta.AuthorPlatform == "" {
		meta.AuthorPlatform = "x"
	}

	if meta.SourceURL != "" {
		if !strings.Contains(meta.SourceURL, "://") {
			meta.SourceURL = "https://" + meta.SourceURL
		}
		if len(meta.SourceURL) > 512 {
			return ImageMeta{}, fmt.Errorf("%w: source url is too long", ErrInvalidInput)
		}
		parsed, err := url.Parse(meta.SourceURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
			return ImageMeta{}, fmt.Errorf("%w: invalid source url", ErrInvalidInput)
		}
		if isXHost(parsed.Hostname()) {
			ref, err := ParseXStatusURL(meta.SourceURL)
			if err != nil {
				return ImageMeta{}, err
			}
			meta.SourceURL = ref.CanonicalURL
			meta.AuthorPlatform = "x"
			if meta.SourcePostID == "" {
				meta.SourcePostID = ref.PostID
			}
			if meta.AuthorHandle == "" {
				meta.AuthorHandle = ref.Handle
			}
		}
	} else {
		// 清空来源时同步清空帖 id，避免留下悬空引用；作者字段独立保留。
		meta.SourcePostID = ""
	}

	return meta, nil
}

// AuthorURL 返回作者主页链接。目前只支持 X；平台或用户名缺失时返回空串。
// 链接由服务端派生而不是落库，避免任意外链进入数据。
func AuthorURL(platform, handle string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if platform == "" && handle != "" {
		platform = "x"
	}
	if platform != "x" || !xHandlePattern.MatchString(handle) {
		return ""
	}
	return "https://x.com/" + handle
}

// ParseXStatusURL 解析并规范化 X/Twitter 状态链接。支持：
//
//	https://x.com/{user}/status/{id}
//	https://twitter.com/{user}/status/{id}/photo/1
//	https://x.com/i/status/{id}
//	https://x.com/i/web/status/{id}
//
// 第三方镜像域（fxtwitter.com 等）使用同样的路径，也一并接受。
func ParseXStatusURL(raw string) (XStatusRef, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return XStatusRef{}, fmt.Errorf("%w: invalid X status url", ErrInvalidInput)
	}
	host := strings.ToLower(parsed.Hostname())
	if !isXHost(host) {
		return XStatusRef{}, fmt.Errorf("%w: not an X status url", ErrInvalidInput)
	}
	if ref, ok := parseXStatusPath(parsed.Path); ok {
		return ref, nil
	}
	return XStatusRef{}, fmt.Errorf("%w: X url does not point to a status", ErrInvalidInput)
}

// parseXStatusPath 从路径中提取用户名与状态 id。
func parseXStatusPath(path string) (XStatusRef, bool) {
	path = strings.TrimRight(path, "/")
	// 先尝试 /i/status、/i/web/status 与根级 /status 形式，避免把 /i/ 误当成
	// 用户名。带用户名的形式不会命中该正则。
	if match := xPostPathRE.FindStringSubmatch(path); len(match) == 2 {
		return XStatusRef{
			CanonicalURL: "https://x.com/i/status/" + match[1],
			PostID:       match[1],
		}, true
	}
	if match := xStatusPathRE.FindStringSubmatch(path); len(match) == 3 {
		return XStatusRef{
			CanonicalURL: "https://x.com/" + match[1] + "/status/" + match[2],
			Handle:       match[1],
			PostID:       match[2],
		}, true
	}
	return XStatusRef{}, false
}

// isXHost 判断主机是否属于受支持的 X/Twitter 域名（含常用镜像域）。
func isXHost(host string) bool {
	switch strings.ToLower(host) {
	case "x.com", "www.x.com", "mobile.x.com",
		"twitter.com", "www.twitter.com", "mobile.twitter.com",
		"fxtwitter.com", "www.fxtwitter.com",
		"vxtwitter.com", "www.vxtwitter.com",
		"fixupx.com", "www.fixupx.com":
		return true
	default:
		return false
	}
}
