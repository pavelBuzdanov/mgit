package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type Shell struct {
	Dir     string
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
}

func NewShell(stdout, stderr io.Writer, verbose bool) *Shell {
	return &Shell{Stdout: stdout, Stderr: stderr, Verbose: verbose}
}

func (s *Shell) Run(ctx context.Context, name string, args []string, extraEnv map[string]string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = s.Dir
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	cmd.Env = mergeEnv(extraEnv)
	if s.Verbose {
		fmt.Fprintf(s.Stderr, "exec: %s %s\n", name, strings.Join(args, " "))
		if len(extraEnv) > 0 {
			fmt.Fprintf(s.Stderr, "env: %s\n", sortedEnvDebug(extraEnv))
		}
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func (s *Shell) Output(ctx context.Context, name string, args []string, extraEnv map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = s.Dir
	cmd.Stderr = s.Stderr
	cmd.Env = mergeEnv(extraEnv)
	var out bytes.Buffer
	cmd.Stdout = &out
	if s.Verbose {
		fmt.Fprintf(s.Stderr, "exec(out): %s %s\n", name, strings.Join(args, " "))
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out.String()), nil
}

func mergeEnv(extra map[string]string) []string {
	base := os.Environ()
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for k, v := range extra {
		prefix := k + "="
		replaced := false
		for i := range out {
			if strings.HasPrefix(out[i], prefix) {
				out[i] = prefix + v
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, prefix+v)
		}
	}
	return out
}

func sortedEnvDebug(extra map[string]string) string {
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+extra[k])
	}
	return strings.Join(parts, " ")
}

func BuildGITSSHCommand(keyPath string) string {
	// GIT_SSH_COMMAND is interpreted by a shell, so single-quote escaping is required.
	// Use -F /dev/null to ignore user-level ~/.ssh/config overrides (Host github.com, IdentityFile, etc.).
	return "ssh -F /dev/null -i " + shellQuote(keyPath) + " -o IdentitiesOnly=yes"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
