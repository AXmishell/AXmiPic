package storage

import (
	"fmt"

	"github.com/AXmishell/axmipic/internal/config"
)

// NewFromConfig 构建由 cfg.Driver 选择的存储后端。
// serverBaseURL 由本地驱动用于构建公开的对象 URL。
func NewFromConfig(cfg config.StorageConfig, serverBaseURL string) (Storage, error) {
	switch cfg.Driver {
	case "local":
		return NewLocal(cfg.Local.Root, serverBaseURL)
	case "s3":
		return NewS3(cfg.S3)
	case "qiniu":
		return NewQiniu(cfg.Qiniu)
	default:
		return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
	}
}
