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

// version is overridden at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() { os.Exit(run()) }

func run() int {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Println("devup", version)
			return 0
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	fs := flag.NewFlagSet("devup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := parser.RegisterFlags(fs)

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
	ports, err := parser.ParsePorts(*cfg.Ports)
	if err != nil {
		logError("Invalid port mapping: %v", err)
		return 2
	}
	if _, ok := parser.ValidSyncModes[*cfg.SyncMode]; !ok {
		logError("Invalid sync mode %q; valid values: two-way-safe, two-way-resolved, one-way-safe, one-way-replica", *cfg.SyncMode)
		return 2
	}
	if err := deps.Check([]string{"ssh", "mutagen"}, exec.LookPath); err != nil {
		logError("Dependency check failed: %v", err)
		return 1
	}
	local, err := parser.ResolveLocalPath(*cfg.LocalPath)
	if err != nil {
		logError("Local path error: %v", err)
		return 1
	}

	sessionName := fmt.Sprintf("devup-%06d", rng.Intn(1000000))
	logInfo("Starting devup session")
	logInfo("Local path:  %s", local)
	logInfo("Remote path: %s:%s", t.Host, t.RemotePath)
	logInfo("Sync mode:   %s", *cfg.SyncMode)

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
	if err := mutagen.CreateSession(sessionName, local, t, ignores, *cfg.SyncMode); err != nil {
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

	sshArgs := sshutil.BuildArgs(ports, t, *cfg.RemoteCmd, *cfg.Identity, logInfo)
	return runSSH(ctx, sshArgs, *cfg.Reconnect, *cfg.RemoteCmd != "")
}

func runSSH(ctx context.Context, sshArgs []string, reconnect bool, hasCmd bool) int {
	for {
		cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
		cmd.Stdout = os.Stdout
		stderrFilter := sshutil.NewStderrFilter(os.Stderr)
		cmd.Stderr = stderrFilter
		cmd.Stdin = os.Stdin

		logInfo("Opening SSH session")
		err := cmd.Run()
		stderrFilter.Close()

		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return 0
		}

		var exitErr *exec.ExitError
		if err == nil {
			// Clean exit: reconnect only if running a command (not interactive shell).
			if !reconnect || !hasCmd {
				return 0
			}
		} else if errors.As(err, &exitErr) {
			if !reconnect {
				logError("SSH session exited with status %d", exitErr.ExitCode())
				return exitErr.ExitCode()
			}
		} else {
			if !reconnect {
				logError("SSH session error: %v", err)
				return 1
			}
		}

		logInfo("SSH disconnected, reconnecting in 3s... (Ctrl+C to stop)")
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return 0
		}
	}
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
	fmt.Fprintln(os.Stderr, "  -p, --port <mapping>       Port forward; repeatable. Formats: 3000 or 3000:3001")
	fmt.Fprintln(os.Stderr, "  -l, --local <path>         Local folder to sync (default: current directory)")
	fmt.Fprintln(os.Stderr, "  -i, --identity <file>      SSH identity file (private key)")
	fmt.Fprintln(os.Stderr, "  --cmd <command>            Command to run on remote after connect")
	fmt.Fprintln(os.Stderr, "  --sync-mode <mode>         Mutagen sync mode (default: one-way-safe)")
	fmt.Fprintln(os.Stderr, "  --reconnect                Auto-reconnect SSH on disconnect")
	fmt.Fprintln(os.Stderr, "  --version                  Print version and exit")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Sync modes: two-way-safe, two-way-resolved, one-way-safe, one-way-replica")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api -p 3000 -p 5173:5174")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api -i ~/.ssh/my_key -l ~/projects/api")
	fmt.Fprintln(os.Stderr, "  devup ubuntu@host:/apps/api --cmd \"npm run dev\" --reconnect")
}

func logInfo(format string, a ...any) { fmt.Printf("[INFO] "+format+"\n", a...) }
func logError(format string, a ...any) { fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", a...) }
