package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type stubWorkflowPathResolver struct {
	path string
}

func (s stubWorkflowPathResolver) ResolvePath(ownerName, repoName string) (string, error) {
	return s.path, nil
}

func TestGetHostContainer_FallsBackWhenRepoHasNoHangrix(t *testing.T) {
	t.Parallel()

	repoFS := initBareTestRepo(t, map[string]string{
		"README.md": "# demo\n",
	})
	svc := New(&Deps{
		PathRes: stubWorkflowPathResolver{path: repoFS},
	})

	container, err := svc.GetHostContainer(context.Background(), Ref{
		ID:            1,
		Name:          "demo",
		OwnerName:     "alice",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("GetHostContainer() error = %v", err)
	}
	if container == nil {
		t.Fatal("GetHostContainer() returned nil container")
	}
	if container.Image != "node:22-bookworm" {
		t.Fatalf("container.Image = %q, want %q", container.Image, "node:22-bookworm")
	}
}

func initBareTestRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	worktree := filepath.Join(root, "work")
	bare := filepath.Join(root, "repo.git")
	mustRunGit(t, root, "init", worktree, "--initial-branch=main")
	mustRunGit(t, worktree, "config", "user.name", "Test User")
	mustRunGit(t, worktree, "config", "user.email", "test@example.com")
	for rel, body := range files {
		full := filepath.Join(worktree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", full, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}
	mustRunGit(t, worktree, "add", ".")
	mustRunGit(t, worktree, "commit", "-m", "init")
	mustRunGit(t, root, "clone", "--bare", worktree, bare)
	return bare
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
