package api

import "net/http"

const (
	listDefaultPageSize = 20
	listMaxPageSize     = 200
)

// pageResponse 是列表接口的分页响应。未传分页参数时，相关接口仍返回旧数组，
// 以兼容既有 SDK 与调用方。
type pageResponse[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// paginationParams 解析 page / page_size，并收敛到安全范围。
func paginationParams(r *http.Request) (page, pageSize int) {
	page = queryInt(r, "page")
	if page < 1 {
		page = 1
	}
	pageSize = queryInt(r, "page_size")
	if pageSize <= 0 {
		pageSize = listDefaultPageSize
	}
	if pageSize > listMaxPageSize {
		pageSize = listMaxPageSize
	}
	return page, pageSize
}
