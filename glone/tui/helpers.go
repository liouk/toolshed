package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func isRepoCloned(cloneDir, name string) bool {
	dest := filepath.Join(cloneDir, name)
	info, err := os.Stat(dest)
	return err == nil && info.IsDir()
}

func httpsToSSH(url string) string {
	url = strings.TrimSuffix(url, ".git")
	if after, ok := strings.CutPrefix(url, "https://github.com/"); ok {
		return "git@github.com:" + after + ".git"
	}
	return url
}

func cloneRepoCmd(url, cloneDir, name string, shallow bool) (string, error) {
	dest := filepath.Join(cloneDir, name)

	if _, err := os.Stat(dest); err == nil {
		return dest, fmt.Errorf("already cloned at %s", dest)
	}

	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		return "", fmt.Errorf("could not create directory: %w", err)
	}

	args := []string{"clone"}
	if shallow {
		args = append(args, "--depth", "1")
	}
	args = append(args, httpsToSSH(url), dest)

	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %s", string(out))
	}
	return dest, nil
}

type cachedRepo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	IsFork      bool   `json:"isFork"`
	ParentOrg   string `json:"parentOrg,omitempty"`
}

func cachePath(org string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "glone", org+".json")
}

func readCache(org string) ([]cachedRepo, error) {
	path := cachePath(org)
	if path == "" {
		return nil, fmt.Errorf("no cache dir")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var repos []cachedRepo
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func writeCache(org string, repos []cachedRepo) {
	path := cachePath(org)
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	data, err := json.Marshal(repos)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0o644)
}

func openEditorCmd(editor, cloneDir, name string) (string, error) {
	dest := filepath.Join(cloneDir, name)

	if editor == "" {
		return dest, nil
	}

	cmd := exec.Command(editor, dest)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not open %s: %w", editor, err)
	}
	return dest, nil
}
