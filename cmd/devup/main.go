package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"devup/internal/cleanup"
	"devup/internal/deps"
	"devup/internal/mutagen"
	"devup/internal/parser"
	sshutil "devup/internal/ssh"
)

func main() { os.Exit(run()) }

func run() int {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	fs := flag.NewFlagSet("devup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	localPath, remoteCmd, portsFlag := parser.RegisterFlags(fs)

	parsedArgs, targetArg, err := parser.SplitArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "[devup] invalid arguments:", err)
		printUsage()
		return 2
	}
	if err := fs.Parse(parsedArgs); err != nil {
		printUsage()
		return 2
	}
	if fs.NArg() != 0 || targetArg == "" {
		printUsage()
		return 2
	}

	t, err := parser.ParseTarget(targetArg)
	if err != nil {
		logError("Invalid target: %v", err)
		printUsage()
		return 2
	}
	ports, err := parser.ParsePorts(*portsFlag)
	if err != nil {
		logError("Invalid port mapping: %v", err)
		return 2
	}
	if err := deps.Check([]string{"ssh", "mutagen"}, exec.LookPath); err != nil {
		logError("Dependency check failed: %v", err)
		return 1
	}
	local, err := parser.ResolveLocalPath(*localPath)
	if err != nil {
		logError("Local path error: %v", err)
		return 1
	}

	sessionName := fmt.Sprintf("devup-%06d", rng.Intn(1000000))
	logInfo("Starting devup session")
	logInfo("Local path:  %s", local)
	logInfo("Remote path: %s:%s", t.Host, t.RemotePath)

	ctx, cancel := cleanup.WithSignals(context.Background(), func() {
		fmt.Println()
		logInfo("Received interrupt, shutting down")
	})
	defer cancel()

	logInfo("Ensuring remote directory exists")
	if err := sshutil.EnsureRemoteDir(ctx, t); err != nil {
		logError("Remote directory setup failed: %v", err)
		return 1
	}
	ignores, err := mutagen.BuildIgnores(local)
	if err != nil {
		logError("Ignore rules error: %v", err)
		return 1
	}
	logInfo("Creating Mutagen sync session")
	if err := mutagen.CreateSession(sessionName, local, t, ignores); err != nil {
		logError("Mutagen session creation failed: %v", err)
		return 1
	}
	defer func() {
		logInfo("Terminating Mutagen sync session")
		if err := mutagen.TerminateSession(sessionName); err != nil {
			logError("Mutagen session termination failed: %v", err)
		}
	}()

	logInfo("Waiting for initial sync to complete")
	if err := mutagen.WaitForSync(ctx, sessionName); err != nil {
		logError("Initial sync failed: %v", err)
		return 1
	}

	sshArgs := sshutil.BuildArgs(ports, t, *remoteCmd, logInfo)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	stderrFilter := sshutil.NewStderrFilter(os.Stderr)
	defer stderrFilter.Close()
	cmd.Stderr = stderrFilter
	cmd.Stdin = os.Stdin

	logInfo("SSH session connected")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.Is(err, context.Canceled) {
			return 0
		}
		if errors.As(err, &exitErr) {
			logError("SSH session exited with status %d", exitErr.ExitCode())
			return exitErr.ExitCode()
		}
		logError("SSH session error: %v", err)
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "devup syncs a local folder to a remote host and opens an SSH dev session.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  devup [user@]host:/remote/path [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "  [user@]host:/remote/path   Remote target (remote path must be absolute)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -p, --port <mapping>   Port forward; repeatable. Formats: 3000 or 3000:3001")
	fmt.Fprintln(os.Stderr, "  -l, --local <path>     Local folder to sync (default: current directory)")
	fmt.Fprintln(os.Stderr, "  --cmd <command>        Command to run on remote after connect")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api -p 3000 -p 5173:5174")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api -l ~/projects/api --cmd \"npm run dev\"")
}

func logInfo(format string, a ...any) { fmt.Printf("[INFO] "+format+"\n", a...) }
func logError(format string, a ...any) { fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", a...) }
