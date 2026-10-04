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

// Local stores objects on the local filesystem rooted at a directory.
type Local struct {
	root    string
	baseURL string
}

// NewLocal creates a local storage driver rooted at root. baseURL is used to
// build public object URLs.
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

// Put writes an object atomically (temp file followed by rename).
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
		// Best-effort cleanup: a successful rename below already moved the file,
		// so this is normally a no-op returning fs.ErrNotExist.
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
		// On Windows Rename fails when the destination exists. Keys are
		// content-addressed, so an existing destination means an identical
		// object was committed concurrently, which satisfies this Put.
		if _, statErr := os.Stat(full); statErr == nil {
			return nil
		}
		return fmt.Errorf("storage: commit object %q: %w", key, err)
	}
	return nil
}

// Get opens an object for reading.
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

// Delete removes an object. Deleting a missing object is a no-op.
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

// Exists reports whether an object is present.
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

// Stat returns metadata for a stored object.
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

// contentTypeFor returns the media type for a stored object. It prefers the
// key extension and falls back to sniffing the file header, so extensionless
// keys still report a correct type.
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

// URL returns the public URL for an object key.
func (l *Local) URL(key string) string {
	return l.baseURL + "/i/" + key
}

// resolve validates key and returns the absolute filesystem path it maps to.
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
