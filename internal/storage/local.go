package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Local 将对象存储在本地文件系统上，根目录由 root 指定。
type Local struct {
	root    string
	baseURL string
}

// NewLocal 创建一个以 root 为根目录的本地存储驱动。baseURL 用于
// 构建公开的对象 URL。
func NewLocal(root, baseURL string) (*Local, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("storage: local root must not be empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve local root %q: %w", root, err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create local root %q: %w", absRoot, err)
	}
	return &Local{root: absRoot, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Put 原子地写入对象（先写临时文件再重命名）。
func (l *Local) Put(_ context.Context, key string, r io.Reader, size int64, _ string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("storage: create directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".axmipic-tmp-*")
	if err != nil {
		return fmt.Errorf("storage: create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// 尽力清理：下面的重命名若成功已移动该文件，
		// 因此这里通常是无操作，返回 fs.ErrNotExist。
		_ = os.Remove(tmpName)
	}()

	written, err := io.Copy(tmp, r)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("storage: write object %q: %w", key, err)
	}
	if size >= 0 && written != size {
		_ = tmp.Close()
		return fmt.Errorf("storage: object %q: wrote %d bytes, expected %d", key, written, size)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("storage: flush object %q: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("storage: close object %q: %w", key, err)
	}

	if err := os.Rename(tmpName, full); err != nil {
		// 在 Windows 上，当目标已存在时 Rename 会失败。键是内容寻址的，
		// 因此目标已存在意味着一个完全相同的对象已被并发提交，
		// 这满足本次 Put。
		if _, statErr := os.Stat(full); statErr == nil {
			return nil
		}
		return fmt.Errorf("storage: commit object %q: %w", key, err)
	}
	return nil
}

// Get 打开一个对象以供读取。
func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("storage: get %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: get %q: %w", key, err)
	}
	return f, nil
}

// Delete 删除一个对象。删除不存在的对象是无操作。
func (l *Local) Delete(_ context.Context, key string) error {
	full, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}

// Exists 报告对象是否存在。
func (l *Local) Exists(_ context.Context, key string) (bool, error) {
	full, err := l.resolve(key)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("storage: stat %q: %w", key, err)
	}
	return true, nil
}

// Stat 返回已存储对象的元数据。
func (l *Local) Stat(_ context.Context, key string) (*ObjectInfo, error) {
	full, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("storage: stat %q: %w", key, ErrNotFound)
		}
		return nil, fmt.Errorf("storage: stat %q: %w", key, err)
	}
	return &ObjectInfo{
		Key:         key,
		Size:        info.Size(),
		ContentType: contentTypeFor(full, path.Ext(key)),
	}, nil
}

// contentTypeFor 返回已存储对象的媒体类型。它优先使用键的扩展名，
// 回退到嗅探文件头，因此无扩展名的键也能报告正确的类型。
func contentTypeFor(full, ext string) string {
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	file, err := os.Open(full)
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()
	head := make([]byte, 512)
	n, _ := file.Read(head)
	if n == 0 {
		return ""
	}
	return http.DetectContentType(head[:n])
}

// URL 返回对象键对应的公开 URL。
func (l *Local) URL(key string) string {
	return l.baseURL + "/i/" + key
}

// List 枚举本地存储中的全部对象（跳过临时文件），供孤儿对象对账使用。
func (l *Local) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	err := filepath.WalkDir(l.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".axmipic-tmp-") {
			return nil
		}
		rel, relErr := filepath.Rel(l.root, p)
		if relErr != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list local objects: %w", err)
	}
	return objects, nil
}

// resolve 校验 key 并返回其映射到的绝对文件系统路径。
func (l *Local) resolve(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: empty key", ErrInvalidKey)
	}
	if strings.ContainsRune(key, '\\') {
		return "", fmt.Errorf("%w: %q contains a backslash", ErrInvalidKey, key)
	}
	if path.IsAbs(key) {
		return "", fmt.Errorf("%w: %q is absolute", ErrInvalidKey, key)
	}
	clean := path.Clean(key)
	if clean != key || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q is not a clean relative path", ErrInvalidKey, key)
	}
	full := filepath.Join(l.root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(l.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes the storage root", ErrInvalidKey, key)
	}
	return full, nil
}
