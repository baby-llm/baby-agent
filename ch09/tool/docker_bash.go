package tool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

const (
	DefaultSandboxContainer = "babyagent-sandbox"
	DefaultSandboxImage     = "alpine:3.19"
)

var invalidContainerChars = regexp.MustCompile(`[^a-z0-9_.-]+`)

// generateContainerName generates a Docker-safe name that is unique per
// workspace path. Using only the basename makes two workspaces with the same
// directory name (e.g. ~/a/project and ~/b/project) share one container.
func generateContainerName(workspaceDir string) string {
	abs, err := filepath.Abs(workspaceDir)
	if err != nil {
		abs = workspaceDir
	}

	projectName := strings.ToLower(filepath.Base(abs))
	projectName = invalidContainerChars.ReplaceAllString(projectName, "-")
	projectName = strings.Trim(projectName, "-_.")
	if projectName == "" || projectName == "." {
		projectName = "workspace"
	}

	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s-%x", DefaultSandboxContainer, projectName, sum[:4])
}

type DockerBashTool struct {
	containerName string
	image         string
	workspaceDir  string

	mu      sync.Mutex
	started bool
}

func NewDockerBashTool(containerName, workspaceDir string) *DockerBashTool {
	if containerName == "" {
		containerName = generateContainerName(workspaceDir)
	}
	return &DockerBashTool{
		containerName: containerName,
		image:         DefaultSandboxImage,
		workspaceDir:  workspaceDir,
	}
}

func (t *DockerBashTool) ToolName() AgentTool {
	return AgentToolBash
}

func (t *DockerBashTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolBash),
		Description: openai.String("execute bash command in a docker sandbox container"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "the bash command to execute in the sandbox",
				},
			},
			"required": []string{"command"},
		},
	})
}

func (t *DockerBashTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	// Lazy initialization: start the container on first use. Only a success
	// is cached, so a transient failure (image pull timeout, ESC) can retry.
	if err := t.ensureStarted(ctx); err != nil {
		return "", fmt.Errorf("failed to start sandbox container: %w", err)
	}

	p := BashToolParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		return "", err
	}

	// Execute command in container via docker exec
	cmd := exec.CommandContext(ctx, "docker", "exec",
		t.containerName,
		"sh", "-c", p.Command)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker exec failed: %w", err)
	}
	return string(output), nil
}

func (t *DockerBashTool) ensureStarted(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return nil
	}
	if err := t.ensureSandboxContainer(ctx); err != nil {
		return err
	}
	t.started = true
	return nil
}

func (t *DockerBashTool) ensureSandboxContainer(ctx context.Context) error {
	// First, try to start existing container
	startCmd := exec.CommandContext(ctx, "docker", "start", t.containerName)
	if startCmd.Run() == nil {
		// Container exists and started successfully
		return nil
	}

	// Container doesn't exist, create new one
	createCmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", t.containerName,
		"--restart", "unless-stopped",
		"-v", t.workspaceDir+":/workspace:rw",
		"-w", "/workspace",
		t.image,
		"sleep", "infinity")

	output, err := createCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create sandbox container: %s: %w", string(output), err)
	}
	return nil
}
