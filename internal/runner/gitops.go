package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type GitOps struct {
	Shell *Shell
}

func NewGitOps(shell *Shell) *GitOps {
	return &GitOps{Shell: shell}
}

func GitInstalled() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	return nil
}

func (g *GitOps) RunGit(ctx context.Context, args []string, extraEnv map[string]string) error {
	return g.Shell.Run(ctx, "git", args, extraEnv)
}

func (g *GitOps) GitOutput(ctx context.Context, args []string, extraEnv map[string]string) (string, error) {
	return g.Shell.Output(ctx, "git", args, extraEnv)
}

func (g *GitOps) GitVersion(ctx context.Context) (string, error) {
	return g.GitOutput(ctx, []string{"--version"}, nil)
}

func (g *GitOps) IsRepo(ctx context.Context) (bool, error) {
	out, err := g.GitOutput(ctx, []string{"rev-parse", "--is-inside-work-tree"}, nil)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

func (g *GitOps) RemoteURL(ctx context.Context, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("empty remote name")
	}
	return g.GitOutput(ctx, []string{"remote", "get-url", name}, nil)
}

func (g *GitOps) Remotes(ctx context.Context) (map[string]string, error) {
	list, err := g.GitOutput(ctx, []string{"remote"}, nil)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, line := range strings.Split(list, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		u, err := g.RemoteURL(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("get URL for remote %q: %w", name, err)
		}
		result[name] = u
	}
	return result, nil
}

func (g *GitOps) CurrentUpstreamRemote(ctx context.Context) (string, error) {
	out, err := g.GitOutput(ctx, []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"}, nil)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return "", fmt.Errorf("could not parse upstream ref %q", out)
	}
	return parts[0], nil
}

func (g *GitOps) GuessDefaultRemote(ctx context.Context) (string, error) {
	if remote, err := g.CurrentUpstreamRemote(ctx); err == nil && remote != "" {
		return remote, nil
	}
	remotes, err := g.Remotes(ctx)
	if err != nil {
		return "", err
	}
	if len(remotes) == 1 {
		for name := range remotes {
			return name, nil
		}
	}
	if _, ok := remotes["origin"]; ok {
		return "origin", nil
	}
	return "", fmt.Errorf("cannot determine default remote automatically")
}
