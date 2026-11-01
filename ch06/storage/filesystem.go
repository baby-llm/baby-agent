package storage

import (
	"context"
	"os"
	"path/filepath"
)

type FileSystemStorage struct {
	baseDir string
}

func NewFileSystemStorage(baseDir string) *FileSystemStorage {
	return &FileSystemStorage{
		baseDir: baseDir,
	}
}

func (fs *FileSystemStorage) Store(ctx context.Context, key string, value string) error {
	filePath := filepath.Clean(filepath.Join(fs.baseDir, key))
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
	filePath := filepath.Clean(filepath.Join(fs.baseDir, key))
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
