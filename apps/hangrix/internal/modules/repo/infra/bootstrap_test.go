package infra

import (
	"testing"

	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/repo/domain"
)

func TestBuildInitialSeedFiles_DefaultTemplate(t *testing.T) {
	repo := &domain.Repo{Name: "demo", Description: "hello"}
	files, err := BuildInitialSeedFiles(repo, true, true, HangrixTemplateDefault)
	if err != nil {
		t.Fatalf("BuildInitialSeedFiles() error = %v", err)
	}
	for _, want := range []string{
		"README.md",
		".hangrix/agents.yml",
		".hangrix/agents/maintainer.md",
		".hangrix/agents/worker.md",
		".hangrix/agents/reviewer.md",
	} {
		if _, ok := files[want]; !ok {
			t.Fatalf("missing seeded file %q", want)
		}
	}
}

func TestBuildInitialSeedFiles_DocsTemplateWithoutReadme(t *testing.T) {
	repo := &domain.Repo{Name: "docs"}
	files, err := BuildInitialSeedFiles(repo, false, true, HangrixTemplateDocs)
	if err != nil {
		t.Fatalf("BuildInitialSeedFiles() error = %v", err)
	}
	if _, ok := files["README.md"]; ok {
		t.Fatal("README.md seeded unexpectedly")
	}
	for _, want := range []string{
		".hangrix/agents.yml",
		".hangrix/agents/maintainer.md",
		".hangrix/agents/worker.md",
		".hangrix/agents/web-reviewer.md",
	} {
		if _, ok := files[want]; !ok {
			t.Fatalf("missing seeded file %q", want)
		}
	}
}

func TestBuildInitialSeedFiles_InvalidTemplate(t *testing.T) {
	repo := &domain.Repo{Name: "demo"}
	if _, err := BuildInitialSeedFiles(repo, true, true, "wat"); err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestBuildInitialSeedFiles_DefaultFallbackWithoutHangrixFiles(t *testing.T) {
	repo := &domain.Repo{Name: "demo", Description: "hello"}
	files, err := BuildInitialSeedFiles(repo, true, false, "")
	if err != nil {
		t.Fatalf("BuildInitialSeedFiles() error = %v", err)
	}
	if _, ok := files["README.md"]; !ok {
		t.Fatal("README.md missing")
	}
	for path := range files {
		if len(path) >= len(".hangrix/") && path[:len(".hangrix/")] == ".hangrix/" {
			t.Fatalf("unexpected .hangrix seed file %q when init_hangrix=false", path)
		}
	}
}
