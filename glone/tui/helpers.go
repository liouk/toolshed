package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func isRepoCloned(cloneDir, name string) bool {
	// A leftover directory from an interrupted clone must remain cloneable.
	// Normal repositories use a .git directory, while linked worktrees use a
	// .git file that points at their shared Git directory.
	_, err := os.Lstat(filepath.Join(cloneDir, name, ".git"))
	return err == nil
}

func cloneRepoCmd(url, cloneDir, name string, shallow bool, report func(string, bool)) (string, error) {
	dest := filepath.Join(cloneDir, name)

	if _, err := os.Stat(dest); err == nil {
		return dest, fmt.Errorf("already cloned at %s", dest)
	}

	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		return "", fmt.Errorf("could not create directory: %w", err)
	}

	// Keep GitHub's HTTPS URL intact. Rewriting it to SSH makes cloning depend
	// on a local SSH setup even though glone already requires authenticated gh.
	args := []string{"repo", "clone", url, dest}
	gitArgs := []string{"--progress"}
	if shallow {
		gitArgs = append(gitArgs, "--depth=1")
	}
	args = append(args, "--")
	args = append(args, gitArgs...)

	cmd := exec.Command("gh", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("could not capture clone output: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("could not capture clone errors: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not start gh repo clone: %w", err)
	}

	var output []string
	var outputMu sync.Mutex
	var readers sync.WaitGroup
	readOutput := func(reader io.Reader) {
		defer readers.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Split(splitProgressLines)
		buffer := make([]byte, 64*1024)
		scanner.Buffer(buffer, 1024*1024)
		for scanner.Scan() {
			raw := scanner.Text()
			replace := strings.HasSuffix(raw, "\r")
			line := strings.TrimRight(raw, "\r\n")
			outputMu.Lock()
			output = append(output, line)
			outputMu.Unlock()
			if report != nil {
				report(line, replace)
			}
		}
	}
	readers.Add(2)
	go readOutput(stdout)
	go readOutput(stderr)

	err = cmd.Wait()
	readers.Wait()
	if err != nil {
		// git creates its destination before contacting the remote. Remove that
		// incomplete checkout so a subsequent attempt is allowed to clone.
		if removeErr := os.RemoveAll(dest); removeErr != nil {
			return "", fmt.Errorf("gh repo clone failed: %s (also could not remove incomplete checkout: %w)", strings.Join(output, "\n"), removeErr)
		}
		return "", fmt.Errorf("gh repo clone failed: %s", strings.Join(output, "\n"))
	}
	return dest, nil
}

func splitProgressLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i+1], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
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
