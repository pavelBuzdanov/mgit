package giturl

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var scpLikeRe = regexp.MustCompile(`^(?:(?P<user>[^@]+)@)?(?P<host>[^:]+):(?P<path>.+)$`)

type Transport string

const (
	TransportSSH   Transport = "ssh"
	TransportHTTPS Transport = "https"
	TransportOther Transport = "other"
)

type ParsedRemote struct {
	Original   string    `json:"original"`
	Transport  Transport `json:"transport"`
	Scheme     string    `json:"scheme,omitempty"`
	User       string    `json:"user,omitempty"`
	Host       string    `json:"host"`
	Port       string    `json:"port,omitempty"`
	Owner      string    `json:"owner,omitempty"` // May contain nested namespaces, e.g. Group/subgroup
	Repo       string    `json:"repo,omitempty"`
	RawPath    string    `json:"rawPath,omitempty"`
	IsRemoteURL bool     `json:"isRemoteURL"`
}

func (p ParsedRemote) IsSSH() bool {
	return p.Transport == TransportSSH
}

func (p ParsedRemote) IsHTTPS() bool {
	return p.Transport == TransportHTTPS
}

func (p ParsedRemote) TargetUserHost() string {
	user := p.User
	if user == "" {
		user = "git"
	}
	return user + "@" + p.Host
}

func Parse(input string) (*ParsedRemote, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, errors.New("empty URL")
	}

	if strings.Contains(s, "://") {
		return parseURL(s)
	}
	return parseSCPLike(s)
}

func IsLikelyRemoteURL(s string) bool {
	if strings.Contains(s, "://") {
		return true
	}
	return scpLikeRe.MatchString(s)
}

func parseURL(raw string) (*ParsedRemote, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("URL %q does not contain host", raw)
	}
	owner, repo, cleanPath, err := splitRepoPath(u.Path)
	if err != nil {
		return nil, fmt.Errorf("parse repository path: %w", err)
	}
	out := &ParsedRemote{
		Original:   raw,
		Scheme:     strings.ToLower(u.Scheme),
		Host:       host,
		Port:       u.Port(),
		User:       "",
		Owner:      owner,
		Repo:       repo,
		RawPath:    cleanPath,
		IsRemoteURL: true,
		Transport:  TransportOther,
	}
	if u.User != nil {
		out.User = u.User.Username()
	}
	switch strings.ToLower(u.Scheme) {
	case "ssh":
		out.Transport = TransportSSH
	case "https":
		out.Transport = TransportHTTPS
	}
	return out, nil
}

func parseSCPLike(raw string) (*ParsedRemote, error) {
	m := scpLikeRe.FindStringSubmatch(raw)
	if m == nil {
		return nil, fmt.Errorf("unsupported remote URL format: %q", raw)
	}
	idx := map[string]int{}
	for i, name := range scpLikeRe.SubexpNames() {
		if name != "" {
			idx[name] = i
		}
	}
	user := m[idx["user"]]
	host := m[idx["host"]]
	rawPath := m[idx["path"]]
	owner, repo, cleanPath, err := splitRepoPath(rawPath)
	if err != nil {
		return nil, fmt.Errorf("parse repository path: %w", err)
	}
	return &ParsedRemote{
		Original:   raw,
		Transport:  TransportSSH,
		Scheme:     "ssh",
		User:       user,
		Host:       host,
		Owner:      owner,
		Repo:       repo,
		RawPath:    cleanPath,
		IsRemoteURL: true,
	}, nil
}

func splitRepoPath(rawPath string) (owner string, repo string, cleanPath string, err error) {
	p := strings.TrimSpace(rawPath)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", "", "", errors.New("repository path is empty")
	}

	segs := strings.Split(p, "/")
	filtered := make([]string, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		filtered = append(filtered, s)
	}
	if len(filtered) < 2 {
		return "", "", "", fmt.Errorf("repository path %q must include owner and repo", rawPath)
	}
	repo = filtered[len(filtered)-1]
	repo = strings.TrimSuffix(repo, ".git")
	if repo == "" {
		return "", "", "", fmt.Errorf("invalid repo in path %q", rawPath)
	}
	owner = path.Clean(strings.Join(filtered[:len(filtered)-1], "/"))
	if owner == "." {
		owner = ""
	}
	if owner == "" {
		return "", "", "", fmt.Errorf("invalid owner/namespace in path %q", rawPath)
	}
	return owner, repo, strings.Join(filtered, "/"), nil
}
