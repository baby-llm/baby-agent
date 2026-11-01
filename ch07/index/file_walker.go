package index

import (
	"os"
	"path/filepath"
	"strings"
)

// FileWalker 负责遍历目录查找文本文件。
//
// 除了构建产物和依赖目录，它还会跳过常见的密钥与本地配置文件。
// 这是一个防误伤清单，不是安全边界：索引前仍应确认工作区里没有
// 你不希望进入向量库的内容。
type FileWalker struct {
	excludeDirs []string
	extensions  []string
}

// NewFileWalker 创建一个新的 FileWalker
func NewFileWalker() *FileWalker {
	return &FileWalker{
		excludeDirs: []string{
			".git", ".idea", ".vscode", "node_modules",
			"vendor", "dist", "build", "target", "bin",
			".venv", "venv", "__pycache__", ".cache",
			".babyagent", ".ssh", ".aws", ".gnupg",
		},
		extensions: []string{
			".go", ".js", ".ts", ".tsx", ".jsx",
			".py", ".java", ".c", ".cpp", ".h", ".hpp",
			".rs", ".rb", ".php", ".cs", ".swift",
			".kt", ".scala", ".sh", ".bash", ".zsh",
			".md", ".txt", ".json", ".yaml", ".yml",
			".toml", ".xml", ".html", ".css", ".scss",
			".sql", ".graphql", ".proto",
		},
	}
}

// WithExcludeDirs 添加需要排除的目录
func (fw *FileWalker) WithExcludeDirs(dirs []string) *FileWalker {
	fw.excludeDirs = append(fw.excludeDirs, dirs...)
	return fw
}

// WithExtensions 添加支持的文件扩展名
func (fw *FileWalker) WithExtensions(exts []string) *FileWalker {
	fw.extensions = append(fw.extensions, exts...)
	return fw
}

// Walk 遍历指定路径，返回所有文本文件的路径
func (fw *FileWalker) Walk(rootPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if fw.ShouldExcludeDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}

		if isSensitiveFile(info.Name()) {
			return nil
		}

		if fw.IsTextFile(path) {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// IsTextFile 检查文件是否是文本文件（通过扩展名）
func (fw *FileWalker) IsTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, supportedExt := range fw.extensions {
		if ext == supportedExt {
			return true
		}
	}
	return false
}

// ShouldExcludeDir 检查目录是否应该被排除
func (fw *FileWalker) ShouldExcludeDir(dirName string) bool {
	for _, excludeDir := range fw.excludeDirs {
		if dirName == excludeDir {
			return true
		}
	}
	return false
}

// isSensitiveFile 判断文件是否可能包含密钥或本地私有配置。
// 索引一个仓库时，把 .env / config.json / 私钥索引进向量库，
// 后续 semantic_search 就可能把它们原样返回给模型。
func isSensitiveFile(name string) bool {
	lower := strings.ToLower(name)

	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if lower == "config.json" ||
		(strings.HasPrefix(lower, "config.") && (strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"))) {
		return true
	}
	switch lower {
	case "credentials", "credentials.json", "secrets", "secrets.json",
		"id_rsa", "id_ed25519", "id_ecdsa":
		return true
	}
	switch filepath.Ext(lower) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	}
	return false
}
