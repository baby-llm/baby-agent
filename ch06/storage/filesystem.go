package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileSystemStorage struct {
	baseDir string
}

func NewFileSystemStorage(baseDir string) *FileSystemStorage {
	return &FileSystemStorage{
		baseDir: baseDir,
	}
}

// resolve converts a storage key (for example "/memory/MEMORY.md") into a path
// inside baseDir and rejects anything that would escape it. Callers include the
// model-controlled load_storage tool, so this check is a security boundary.
func (fs *FileSystemStorage) resolve(key string) (string, error) {
	base, err := filepath.Abs(fs.baseDir)
	if err != nil {
		return "", err
	}
	relKey := strings.TrimPrefix(filepath.ToSlash(key), "/")
	filePath := filepath.Join(base, filepath.FromSlash(relKey))

	rel, err := filepath.Rel(base, filePath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key %q escapes base directory", key)
	}
	return filePath, nil
}

func (fs *FileSystemStorage) Store(ctx context.Context, key string, value string) error {
	filePath, err := fs.resolve(key)
	if err != nil {
		return err
	}

	// 如果目录不存在，创建目录
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// 先写临时文件再 rename，避免进程中断时留下半个 MEMORY.md。
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后这里删除的是不存在的旧临时名

	if _, err := tmp.WriteString(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filePath)
}

func (fs *FileSystemStorage) Load(ctx context.Context, key string) (string, error) {
	filePath, err := fs.resolve(key)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
