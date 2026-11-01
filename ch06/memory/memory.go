package memory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"babyagent/ch06/storage"
	"babyagent/shared"
)

type Memory interface {
	String() string
	Update(ctx context.Context, newMessages []shared.OpenAIMessage) error
}

type MultiLevelMemory struct {
	content MemoryContent

	globalStorage    storage.Storage
	globalKey        string
	workspaceStorage storage.Storage
	workspaceKey     string

	updater MemoryUpdater
}

func NewMultiLevelMemory(globalStorage storage.Storage, workspaceStorage storage.Storage, u MemoryUpdater) (*MultiLevelMemory, error) {
	m := &MultiLevelMemory{
		globalStorage:    globalStorage,
		globalKey:        "/memory/MEMORY.md",
		workspaceStorage: workspaceStorage,
		workspaceKey:     "/memory/MEMORY.md",
		updater:          u,
	}
	content, err := m.load()
	if err != nil {
		return nil, err
	}
	m.content = content
	return m, nil
}

// load 只把“文件不存在”当作空记忆。权限错误或文件损坏必须上报，
// 否则下一轮 Update 会把这个空内容写回磁盘，覆盖真正的记忆。
func (m *MultiLevelMemory) load() (MemoryContent, error) {
	ctx := context.Background()
	content := MemoryContent{}

	workspace, err := m.workspaceStorage.Load(ctx, m.workspaceKey)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return MemoryContent{}, fmt.Errorf("load workspace memory: %w", err)
	}
	global, err := m.globalStorage.Load(ctx, m.globalKey)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return MemoryContent{}, fmt.Errorf("load global memory: %w", err)
	}

	content.WorkspaceMemory = workspace
	content.GlobalMemory = global
	return content, nil
}

func (m *MultiLevelMemory) String() string {
	return m.content.String()
}

func (m *MultiLevelMemory) Update(ctx context.Context, newMessages []shared.OpenAIMessage) error {
	if len(newMessages) == 0 {
		return nil
	}

	newMemory, err := m.updater.Update(ctx, m.content, newMessages)
	if err != nil {
		return err
	}

	oldMemory := m.content
	if err := m.globalStorage.Store(ctx, m.globalKey, newMemory.GlobalMemory); err != nil {
		return err
	}
	if err := m.workspaceStorage.Store(ctx, m.workspaceKey, newMemory.WorkspaceMemory); err != nil {
		// 第二层写入失败时把第一层恢复，避免磁盘和内存长期不一致。
		_ = m.globalStorage.Store(ctx, m.globalKey, oldMemory.GlobalMemory)
		return err
	}

	// 两个文件都写成功后再更新内存中的内容。
	m.content = newMemory
	return nil
}

type MemoryContent struct {
	GlobalMemory    string `json:"global_memory,omitempty"`
	WorkspaceMemory string `json:"workspace_memory,omitempty"`
}

func (m *MemoryContent) String() string {
	prompt := memoryPromptTemplate
	prompt = strings.ReplaceAll(prompt, "{global_memory}", m.GlobalMemory)
	prompt = strings.ReplaceAll(prompt, "{workspace_memory}", m.WorkspaceMemory)
	return prompt
}

const memoryPromptTemplate = `### Global Memory
Here is the memory about the user among all conversations:
{global_memory}

### Workspace Memory
The memory of the current workspace is:
{workspace_memory}
`
