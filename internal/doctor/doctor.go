package doctor

import (
	"context"
	"fmt"
	"sort"

	"mgit/internal/config"
	"mgit/internal/resolve"
	"mgit/internal/runner"
)

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok|warn|error
	Message string `json:"message"`
}

type RemoteReport struct {
	Name    string          `json:"name"`
	URL     string          `json:"url"`
	Result  *resolve.Result `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Warning string          `json:"warning,omitempty"`
}

type Report struct {
	ConfigPath    string                   `json:"configPath"`
	Checks        []Check                  `json:"checks"`
	ConfigIssues  []config.ValidationIssue `json:"configIssues,omitempty"`
	Remotes       []RemoteReport           `json:"remotes,omitempty"`
	Unmatched     []string                 `json:"unmatchedRemotes,omitempty"`
	GitVersion    string                   `json:"gitVersion,omitempty"`
	IsGitRepo     bool                     `json:"isGitRepo"`
	ConfigLoaded  bool                     `json:"configLoaded"`
}

func Build(ctx context.Context, git *runner.GitOps, cfg *config.Config, cfgPath string) Report {
	rep := Report{ConfigPath: cfgPath}

	if err := runner.GitInstalled(); err != nil {
		rep.Checks = append(rep.Checks, Check{Name: "git", Status: "error", Message: err.Error()})
	} else {
		ver, err := git.GitVersion(ctx)
		if err != nil {
			rep.Checks = append(rep.Checks, Check{Name: "git", Status: "warn", Message: err.Error()})
		} else {
			rep.GitVersion = ver
			rep.Checks = append(rep.Checks, Check{Name: "git", Status: "ok", Message: ver})
		}
	}

	if cfg != nil {
		rep.ConfigLoaded = true
		issues := cfg.Validate()
		rep.ConfigIssues = issues
		if config.HasErrors(issues) {
			rep.Checks = append(rep.Checks, Check{Name: "config", Status: "error", Message: "config validation failed"})
		} else if len(issues) > 0 {
			rep.Checks = append(rep.Checks, Check{Name: "config", Status: "warn", Message: "config has warnings"})
		} else {
			rep.Checks = append(rep.Checks, Check{Name: "config", Status: "ok", Message: "config is valid"})
		}
	} else {
		rep.Checks = append(rep.Checks, Check{Name: "config", Status: "error", Message: "config not loaded"})
	}

	isRepo, err := git.IsRepo(ctx)
	if err != nil {
		rep.IsGitRepo = false
		rep.Checks = append(rep.Checks, Check{Name: "repo", Status: "warn", Message: "not a git repository (or git unavailable in current directory)"})
		return rep
	}
	rep.IsGitRepo = isRepo
	if !isRepo {
		rep.Checks = append(rep.Checks, Check{Name: "repo", Status: "warn", Message: "current directory is not a git repository"})
		return rep
	}
	rep.Checks = append(rep.Checks, Check{Name: "repo", Status: "ok", Message: "inside git repository"})

	remotes, err := git.Remotes(ctx)
	if err != nil {
		rep.Checks = append(rep.Checks, Check{Name: "remotes", Status: "error", Message: fmt.Sprintf("failed to read remotes: %v", err)})
		return rep
	}
	if len(remotes) == 0 {
		rep.Checks = append(rep.Checks, Check{Name: "remotes", Status: "warn", Message: "no remotes configured"})
		return rep
	}
	rep.Checks = append(rep.Checks, Check{Name: "remotes", Status: "ok", Message: fmt.Sprintf("%d remote(s) found", len(remotes))})

	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		url := remotes[name]
		rr := RemoteReport{Name: name, URL: url}
		if cfg == nil {
			rr.Warning = "config not loaded"
			rep.Remotes = append(rep.Remotes, rr)
			continue
		}
		res, err := resolve.FromURL(cfg, url)
		if err != nil {
			rr.Error = err.Error()
			rep.Unmatched = append(rep.Unmatched, name)
		} else {
			rr.Result = res
		}
		rep.Remotes = append(rep.Remotes, rr)
	}
	return rep
}
