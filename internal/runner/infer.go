package runner

import (
	"fmt"
	"strings"

	"mgit/internal/giturl"
)

type TargetKind string

const (
	TargetNone   TargetKind = "none"
	TargetRemote TargetKind = "remote"
	TargetURL    TargetKind = "url"
)

type GitTarget struct {
	Kind       TargetKind `json:"kind"`
	Command    string     `json:"command,omitempty"`
	RemoteName string     `json:"remoteName,omitempty"`
	URL        string     `json:"url,omitempty"`
	Notes      string     `json:"notes,omitempty"`
	SkipSSHSelection bool `json:"skipSshSelection,omitempty"`
}

func InferGitTarget(args []string) (GitTarget, error) {
	if len(args) == 0 {
		return GitTarget{Kind: TargetNone}, nil
	}
	cmd := args[0]
	switch cmd {
	case "push", "fetch", "pull":
		pos := positionalArgs(args[1:])
		if len(pos) == 0 {
			return GitTarget{Kind: TargetNone, Command: cmd, Notes: "remote not specified explicitly"}, nil
		}
		if giturl.IsLikelyRemoteURL(pos[0]) {
			return GitTarget{Kind: TargetURL, Command: cmd, URL: pos[0]}, nil
		}
		return GitTarget{Kind: TargetRemote, Command: cmd, RemoteName: pos[0]}, nil
	case "clone":
		pos := positionalArgs(args[1:])
		if len(pos) == 0 {
			return GitTarget{Kind: TargetNone, Command: cmd}, fmt.Errorf("clone requires repository URL")
		}
		return GitTarget{Kind: TargetURL, Command: cmd, URL: pos[0]}, nil
	case "ls-remote":
		pos := positionalArgs(args[1:])
		if len(pos) == 0 {
			return GitTarget{Kind: TargetNone, Command: cmd, Notes: "no repository argument"}, nil
		}
		if giturl.IsLikelyRemoteURL(pos[0]) {
			return GitTarget{Kind: TargetURL, Command: cmd, URL: pos[0]}, nil
		}
		return GitTarget{Kind: TargetRemote, Command: cmd, RemoteName: pos[0]}, nil
	case "remote":
		if len(args) >= 2 && args[1] == "set-url" {
			pos := positionalArgs(args[2:])
			// git remote set-url [--push] <name> <newurl> [<oldurl>]
			if len(pos) >= 2 {
				if giturl.IsLikelyRemoteURL(pos[1]) {
					return GitTarget{
						Kind:             TargetURL,
						Command:          "remote set-url",
						URL:              pos[1],
						Notes:            "local config update; SSH key selection not required",
						SkipSSHSelection: true,
					}, nil
				}
			}
		}
	}
	return GitTarget{Kind: TargetNone, Command: cmd}, nil
}

func positionalArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i+1:]...)
			break
		}
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			if takesValue(a) && i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func takesValue(flag string) bool {
	if strings.Contains(flag, "=") {
		return false
	}
	switch flag {
	case "-c", "--config", "-C", "--upload-pack", "--receive-pack", "-o":
		return true
	default:
		return false
	}
}
