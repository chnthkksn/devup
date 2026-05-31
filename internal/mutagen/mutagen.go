package mutagen

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devup/internal/parser"
)

var DefaultIgnores = []string{".git", "node_modules", ".next", ".dist", "coverage"}

func BuildIgnores(local string) ([]string, error) {
	merged := make([]string, 0, len(DefaultIgnores)+16)
	seen := make(map[string]struct{}, len(DefaultIgnores)+16)

	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}

	for _, ig := range DefaultIgnores {
		add(ig)
	}

	f, err := os.Open(filepath.Join(local, ".gitignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return merged, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Negation patterns are include rules; skip them because mutagen --ignore accepts excludes.
		if strings.HasPrefix(line, "!") {
			continue
		}
		add(line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return merged, nil
}

func CreateSession(name, local string, t parser.Target, ignores []string) error {
	args := []string{"sync", "create", "--name", name}
	for _, ig := range ignores {
		args = append(args, "--ignore="+ig)
	}
	args = append(args, local, fmt.Sprintf("%s:%s", t.Host, t.RemotePath))
	cmd := exec.Command("mutagen", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func WaitForSync(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "mutagen", "sync", "flush", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TerminateSession(name string) error {
	return exec.Command("mutagen", "sync", "terminate", name).Run()
}
