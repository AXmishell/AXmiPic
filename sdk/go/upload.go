package axmipic

import (
	"bytes"
	"context"
	"net/http"
)

// Upload 通过 multipart 上传一张图片。
func (c *Client) Upload(ctx context.Context, filename string, data []byte) (*Image, error) {
	body, contentType, err := newUploadBody("file", filename, data)
	if err != nil {
		return nil, err
	}
	var out Image
	if err := c.Do(ctx, http.MethodPost, "/api/v1/upload", body.Bytes(), contentType, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Presign 申请对象存储预签名直传。
func (c *Client) Presign(ctx context.Context, mimeType string, size int64) (*PresignResult, error) {
	var out PresignResult
	in := map[string]any{"mime_type": mimeType, "size": size}
	if err := c.postJSON(ctx, "/api/v1/upload/presign", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Confirm 确认预签名直传完成并入库。
func (c *Client) Confirm(ctx context.Context, key string) (*Image, error) {
	var out Image
	if err := c.postJSON(ctx, "/api/v1/upload/confirm", map[string]string{"key": key}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutPresigned 根据 Presign 结果把数据直传到存储，适用于 S3/七牛等的直传。
// 对于使用表单（Fields）的直传，会自动构造 multipart 表单。
func (c *Client) PutPresigned(ctx context.Context, result *PresignResult, data []byte) error {
	if result == nil || result.UploadURL == "" {
		return &Error{Code: -1, Message: "presign result is empty"}
	}
	if len(result.Fields) > 0 {
		body, contentType, err := multipartFields(result.Fields, "file", data)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, result.UploadURL, body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return &Error{Code: -1, Message: err.Error()}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return &Error{StatusCode: resp.StatusCode, Code: resp.StatusCode, Message: "presigned upload failed"}
		}
		return nil
	}
	method := result.Method
	if method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequestWithContext(ctx, method, result.UploadURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	for k, v := range result.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Code: -1, Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &Error{StatusCode: resp.StatusCode, Code: resp.StatusCode, Message: "presigned upload failed"}
	}
	return nil
}
