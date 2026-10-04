package axmipic

import (
	"context"
	"strconv"
)

// ImageQuery 约束图片列表查询。
type ImageQuery struct {
	Page       int
	PageSize   int
	Order      string
	Keyword    string
	AlbumID    string
	Permission string
	UserID     string
}

func (q ImageQuery) params() map[string]string {
	params := map[string]string{
		"order":      q.Order,
		"keyword":    q.Keyword,
		"album_id":   q.AlbumID,
		"permission": q.Permission,
		"user_id":    q.UserID,
	}
	if q.Page > 0 {
		params["page"] = strconv.Itoa(q.Page)
	}
	if q.PageSize > 0 {
		params["page_size"] = strconv.Itoa(q.PageSize)
	}
	return params
}

// ListImages 返回当前账号的图片列表。
func (c *Client) ListImages(ctx context.Context, query ImageQuery) (*ImageList, error) {
	var out ImageList
	if err := c.get(ctx, encodeQuery("/api/v1/images", query.params()), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetImage 返回单张图片。
func (c *Client) GetImage(ctx context.Context, id string) (*Image, error) {
	var out Image
	if err := c.get(ctx, "/api/v1/images/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RenameImage 修改图片的展示名。
func (c *Client) RenameImage(ctx context.Context, id, name string) (*Image, error) {
	var out Image
	if err := c.patchJSON(ctx, "/api/v1/images/"+id, map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteImage 删除一张图片。
func (c *Client) DeleteImage(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/images/"+id, nil)
}

// BatchUpdate 对一组图片批量设置可见性或所属相册。
type BatchUpdate struct {
	IDs        []string `json:"ids"`
	Permission string   `json:"permission,omitempty"`
	AlbumID    *string  `json:"album_id,omitempty"`
	ClearAlbum bool     `json:"clear_album,omitempty"`
}

// BatchImages 批量更新图片，返回受影响的数量。
func (c *Client) BatchImages(ctx context.Context, update BatchUpdate) (int, error) {
	var out struct {
		Updated int `json:"updated"`
	}
	if err := c.postJSON(ctx, "/api/v1/images/batch", update, &out); err != nil {
		return 0, err
	}
	return out.Updated, nil
}

// ListPlaza 返回公开图片广场的一页图片。
func (c *Client) ListPlaza(ctx context.Context, query ImageQuery) (*ImageList, error) {
	var out ImageList
	if err := c.get(ctx, encodeQuery("/api/v1/plaza", query.params()), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPublicAlbums 返回公开相册，可按所有者过滤。
func (c *Client) ListPublicAlbums(ctx context.Context, userID string) ([]Album, error) {
	var out []Album
	path := encodeQuery("/api/v1/plaza/albums", map[string]string{"user_id": userID})
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPublicProfile 返回用户的公开资料。
func (c *Client) GetPublicProfile(ctx context.Context, userID string) (*PublicProfile, error) {
	var out PublicProfile
	if err := c.get(ctx, "/api/v1/users/"+userID, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TransformParams 是即时图片处理参数。零值字段会被忽略。
type TransformParams struct {
	Width   int
	Height  int
	Fit     string
	Quality int
	Format  string
	Rotate  int
	Flip    string
	Gray    bool
	Blur    float64
	Sharpen float64
	Enlarge bool

	Watermark         string
	WatermarkPosition string
	WatermarkOpacity  int
	WatermarkSize     int
	WatermarkColor    string
}

// TransformURL 在图片 URL 上附加处理参数，返回处理后的 URL。
func (p TransformParams) TransformURL(baseURL string) string {
	params := map[string]string{}
	setInt := func(key string, v int) {
		if v != 0 {
			params[key] = strconv.Itoa(v)
		}
	}
	setInt("w", p.Width)
	setInt("h", p.Height)
	setInt("q", p.Quality)
	setInt("r", p.Rotate)
	if p.Fit != "" {
		params["fit"] = p.Fit
	}
	if p.Format != "" {
		params["f"] = p.Format
	}
	if p.Flip != "" {
		params["flip"] = p.Flip
	}
	if p.Gray {
		params["gray"] = "1"
	}
	if p.Blur != 0 {
		params["blur"] = strconv.FormatFloat(p.Blur, 'f', -1, 64)
	}
	if p.Sharpen != 0 {
		params["sharpen"] = strconv.FormatFloat(p.Sharpen, 'f', -1, 64)
	}
	if p.Enlarge {
		params["enlarge"] = "1"
	}
	if p.Watermark != "" {
		params["wm"] = p.Watermark
		if p.WatermarkPosition != "" {
			params["wm_pos"] = p.WatermarkPosition
		}
		if p.WatermarkOpacity != 0 {
			params["wm_opacity"] = strconv.Itoa(p.WatermarkOpacity)
		}
		if p.WatermarkSize != 0 {
			params["wm_size"] = strconv.Itoa(p.WatermarkSize)
		}
		if p.WatermarkColor != "" {
			params["wm_color"] = p.WatermarkColor
		}
	}
	return encodeQuery(baseURL, params)
}
