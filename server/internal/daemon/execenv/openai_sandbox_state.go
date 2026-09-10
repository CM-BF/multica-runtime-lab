package execenv

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/multica-ai/multica/server/internal/cli"
)

// PrepareOpenAISandboxState pins checkpoints to a private conversation shard.
// No configuration, credentials or database is copied from a global profile.
func PrepareOpenAISandboxState(profile, runtimeID, agentID, workspaceID, taskID string, task TaskContextForEnv) (string, error) {
	root, err := cli.ProfileDir(profile)
	if err != nil {
		return "", err
	}
	return prepareOpenAISandboxStateAt(root, runtimeID, agentID, workspaceID, taskID, task)
}
func prepareOpenAISandboxStateAt(root, runtimeID, agentID, workspaceID, taskID string, task TaskContextForEnv) (string, error) {
	conversation := hermesConversationSegment(task)
	if conversation == "" {
		conversation = "task_" + taskID
	}
	if runtimeID == "" || agentID == "" || conversation == "task_" {
		return "", fmt.Errorf("openai-sandbox: runtime, agent and conversation identity required")
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(workspaceID+"\x00"+runtimeID+"\x00"+agentID+"\x00"+conversation)))
	path := filepath.Join(root, "openai-sandbox-state", key)
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0700); err != nil {
		return "", err
	}
	return path, nil
}
