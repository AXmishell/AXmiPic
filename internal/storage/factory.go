package storage

import (
	"fmt"

	"github.com/axmipic/axmipic/internal/config"
)

// NewFromConfig builds the storage backend selected by cfg.Driver.
// serverBaseURL is used by the local driver to build public object URLs.
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
