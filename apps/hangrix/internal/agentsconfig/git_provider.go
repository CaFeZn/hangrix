package agentsconfig

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

// GitFileProvider reads config files directly from a bare git repo at a fixed
// ref. It lets modules share the same LoadHostConfig fallback semantics
// without importing each other's helper packages.
type GitFileProvider struct {
	Ctx        context.Context
	RepoFSPath string
	Ref        string
}

func (p *GitFileProvider) ReadFile(path string) ([]byte, bool) {
	cmd := exec.CommandContext(p.Ctx,
		"git",
		"--git-dir="+p.RepoFSPath,
		"cat-file",
		"-p",
		p.Ref+":"+path,
	)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

func (p *GitFileProvider) ListDir(dir string) ([]string, bool) {
	cmd := exec.CommandContext(p.Ctx,
		"git",
		"--git-dir="+p.RepoFSPath,
		"ls-tree",
		"--name-only",
		p.Ref,
		strings.TrimSuffix(dir, "/")+"/",
	)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	if len(paths) == 0 {
		return nil, false
	}
	return paths, true
}

var _ FileProvider = (*GitFileProvider)(nil)
