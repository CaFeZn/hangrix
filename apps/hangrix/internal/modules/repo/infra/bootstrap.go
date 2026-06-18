package infra

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/repo/domain"
)

const (
	HangrixTemplateDefault = "default"
	HangrixTemplateDocs    = "docs"
)

//go:embed templates/default/* templates/default/agents/* templates/docs/* templates/docs/agents/*
var hangrixTemplateFS embed.FS

// BuildInitialSeedFiles returns the files that should exist in the repository's
// initial commit based on repo-creation options.
func BuildInitialSeedFiles(repo *domain.Repo, seedReadme bool, initHangrix bool, hangrixTemplate string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if seedReadme {
		description := repo.Description
		if description == "" {
			description = "This repository was created by Hangrix."
		}
		files["README.md"] = []byte(fmt.Sprintf("# %s\n\n%s\n", repo.Name, description))
	}
	if !initHangrix {
		return files, nil
	}
	preset, err := normalizeHangrixTemplate(hangrixTemplate)
	if err != nil {
		return nil, err
	}
	root := path.Join("templates", preset)
	var paths []string
	if err := fs.WalkDir(hangrixTemplateFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk hangrix template %q: %w", preset, err)
	}
	sort.Strings(paths)
	for _, p := range paths {
		body, err := hangrixTemplateFS.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read hangrix template file %q: %w", p, err)
		}
		rel := strings.TrimPrefix(p, root+"/")
		files[path.Join(".hangrix", path.Clean(rel))] = body
	}
	return files, nil
}

func normalizeHangrixTemplate(v string) (string, error) {
	switch strings.TrimSpace(v) {
	case "", HangrixTemplateDefault:
		return HangrixTemplateDefault, nil
	case HangrixTemplateDocs:
		return HangrixTemplateDocs, nil
	default:
		return "", fmt.Errorf("unknown hangrix template %q", v)
	}
}
