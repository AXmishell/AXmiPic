package axmipic

import (
	"context"
	"strconv"
)

// AlbumInput 是创建或更新相册的输入。
type AlbumInput struct {
	Name       string `json:"name"`
	Intro      string `json:"intro"`
	Permission string `json:"permission,omitempty"`
}

// ListAlbums 返回当前账号的相册。
func (c *Client) ListAlbums(ctx context.Context) ([]Album, error) {
	var out []Album
	if err := c.get(ctx, "/api/v1/albums", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAlbum 新建相册。
func (c *Client) CreateAlbum(ctx context.Context, in AlbumInput) (*Album, error) {
	var out Album
	if err := c.postJSON(ctx, "/api/v1/albums", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAlbum 返回相册详情。
func (c *Client) GetAlbum(ctx context.Context, id string) (*Album, error) {
	var out Album
	if err := c.get(ctx, "/api/v1/albums/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAlbum 修改相册。
func (c *Client) UpdateAlbum(ctx context.Context, id string, in AlbumInput) (*Album, error) {
	var out Album
	if err := c.patchJSON(ctx, "/api/v1/albums/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAlbum 删除相册（图片保留）。
func (c *Client) DeleteAlbum(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/albums/"+id, nil)
}

// ListAlbumImages 返回相册中的图片。
func (c *Client) ListAlbumImages(ctx context.Context, id string, page, pageSize int) (*ImageList, error) {
	params := map[string]string{}
	if page > 0 {
		params["page"] = strconv.Itoa(page)
	}
	if pageSize > 0 {
		params["page_size"] = strconv.Itoa(pageSize)
	}
	var out ImageList
	if err := c.get(ctx, encodeQuery("/api/v1/albums/"+id+"/images", params), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
