package parser

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PortMapping struct {
	Local  int
	Remote int
}

type Target struct {
	Host       string
	RemotePath string
}

type MultiFlag []string

func (m *MultiFlag) String() string { return strings.Join(*m, ",") }
func (m *MultiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// Flag name constants — used in both RegisterFlags and valueTakingFlags.
const (
	flagPort  = "p"
	flagPortL = "port"
	flagLocal = "l"
	flagLocalL = "local"
	flagCmd   = "cmd"
)

// valueTakingFlags is the set of flags that consume the next argument as their
// value. Must stay consistent with RegisterFlags.
var valueTakingFlags = map[string]struct{}{
	"-" + flagPort:    {},
	"--" + flagPortL:  {},
	"-" + flagLocal:   {},
	"--" + flagLocalL: {},
	"--" + flagCmd:    {},
}

func RegisterFlags(fs *flag.FlagSet) (*string, *string, *MultiFlag) {
	local := fs.String(flagLocal, "", "Local folder")
	fs.StringVar(local, flagLocalL, "", "Local folder")
	cmd := fs.String(flagCmd, "", "Remote startup command")
	var ports MultiFlag
	fs.Var(&ports, flagPort, "Port mapping")
	fs.Var(&ports, flagPortL, "Port mapping")
	return local, cmd, &ports
}

func SplitArgs(args []string) ([]string, string, error) {
	var target string
	parsed := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			parsed = append(parsed, arg)
			_, takesValue := valueTakingFlags[arg]
			if takesValue && !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return nil, "", fmt.Errorf("flag %q requires a value", arg)
				}
				i++
				parsed = append(parsed, args[i])
			}
			continue
		}
		if target == "" {
			target = arg
			continue
		}
		return nil, "", errors.New("multiple positional arguments provided")
	}
	return parsed, target, nil
}

func ParseTarget(input string) (Target, error) {
	parts := strings.SplitN(input, ":", 2)
	if len(parts) != 2 {
		return Target{}, errors.New("expected [user@]host:/remote/path")
	}
	host := strings.TrimSpace(parts[0])
	remotePath := strings.TrimSpace(parts[1])
	if host == "" || remotePath == "" {
		return Target{}, errors.New("host and remote path must be non-empty")
	}
	if !strings.HasPrefix(remotePath, "/") {
		return Target{}, errors.New("remote path must be absolute")
	}
	return Target{Host: host, RemotePath: remotePath}, nil
}

func ParsePorts(values []string) ([]PortMapping, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]PortMapping, 0, len(values))
	for _, v := range values {
		p, err := ParsePortMapping(v)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func ParsePortMapping(input string) (PortMapping, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return PortMapping{}, errors.New("empty mapping")
	}
	parts := strings.SplitN(input, ":", 2)
	if len(parts) == 1 {
		p, err := parseSinglePort(parts[0])
		if err != nil {
			return PortMapping{}, err
		}
		return PortMapping{Local: p, Remote: p}, nil
	}
	local, err := parseSinglePort(parts[0])
	if err != nil {
		return PortMapping{}, fmt.Errorf("local port: %w", err)
	}
	remote, err := parseSinglePort(parts[1])
	if err != nil {
		return PortMapping{}, fmt.Errorf("remote port: %w", err)
	}
	return PortMapping{Local: local, Remote: remote}, nil
}

func ResolveLocalPath(input string) (string, error) {
	path := input
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get cwd: %w", err)
		}
		path = wd
	}
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return abs, nil
}

func parseSinglePort(input string) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, errors.New("port cannot be empty")
	}
	p, err := strconv.Atoi(input)
	if err != nil {
		return 0, errors.New("port must be numeric")
	}
	if p < 1 || p > 65535 {
		return 0, errors.New("port must be in range 1-65535")
	}
	return p, nil
}

