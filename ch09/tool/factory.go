package tool

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// checkDockerAvailable checks whether the docker CLI and daemon respond.
// A short timeout matters: a stuck daemon must not hang agent startup.
func checkDockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "ps").Run() == nil
}

// CreateBashTool creates a bash tool, automatically choosing between
// DockerBashTool (if docker is available) and regular BashTool.
// The returned error is a warning: the caller gets a working tool either way,
// but should surface that sandbox isolation is not active.
func CreateBashTool(workspaceDir string) (Tool, error) {
	if !checkDockerAvailable() {
		return NewBashTool(), errors.New("docker unavailable; falling back to host bash (no sandbox isolation)")
	}
	if workspaceDir == "" {
		return NewBashTool(), errors.New("workspace dir is empty; falling back to host bash (no sandbox isolation)")
	}
	return NewDockerBashTool("", workspaceDir), nil
}
