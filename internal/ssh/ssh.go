package ssh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"devup/internal/parser"
)

func BuildArgs(ports []parser.PortMapping, t parser.Target, remoteCmd string, logf func(string, ...any)) []string {
	args := make([]string, 0, len(ports)*2+4)
	for _, m := range ports {
		if logf != nil {
			logf("Port forward: localhost:%d -> remote:%d", m.Local, m.Remote)
		}
		args = append(args, "-L", fmt.Sprintf("%d:localhost:%d", m.Local, m.Remote))
	}
	if remoteCmd != "" {
		args = append(args, t.Host, fmt.Sprintf("cd %s && %s", shellQuote(t.RemotePath), remoteCmd))
		return args
	}
	args = append(args, "-t", t.Host, fmt.Sprintf("cd %s && exec ${SHELL:-/bin/bash} -l", shellQuote(t.RemotePath)))
	return args
}

func EnsureRemoteDir(ctx context.Context, t parser.Target) error {
	cmd := exec.CommandContext(ctx, "ssh", t.Host, "mkdir -p "+shellQuote(t.RemotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

