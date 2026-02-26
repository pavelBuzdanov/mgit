package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const CurrentVersion = 1
const RepoConfigRelativePath = ".mgit/config.json"

type Config struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

type Rule struct {
	ID       string `json:"id,omitempty"`
	Host     string `json:"host"`
	Owner    string `json:"owner"`
	Key      string `json:"key"`
	Priority int    `json:"priority,omitempty"`
}

type ValidationIssue struct {
	Level   string `json:"level"` // error|warning
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type RemoveSelector struct {
	ID    string
	Host  string
	Owner string
	Key   string
	Index int // 1-based, <=0 ignored
}

func GlobalDefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine user config dir: %w", err)
	}
	return filepath.Join(dir, "mgit", "config.json"), nil
}

func DefaultPath() (string, error) {
	return AutoPath()
}

func ResolvePath(custom string) (string, error) {
	if strings.TrimSpace(custom) == "" {
		return AutoPath()
	}
	return ExpandPath(custom)
}

func AutoPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("determine current working directory: %w", err)
	}
	if p, ok, err := FindNearestConfig(wd); err == nil && ok {
		return p, nil
	} else if err != nil {
		return "", err
	}
	if repoRoot, ok, err := FindRepoRoot(wd); err == nil && ok {
		return filepath.Join(repoRoot, RepoConfigRelativePath), nil
	} else if err != nil {
		return "", err
	}
	return filepath.Join(wd, RepoConfigRelativePath), nil
}

func FindNearestConfig(start string) (string, bool, error) {
	dir, err := ExpandPath(start)
	if err != nil {
		return "", false, err
	}
	for {
		candidate := filepath.Join(dir, RepoConfigRelativePath)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func FindRepoRoot(start string) (string, bool, error) {
	dir, err := ExpandPath(start)
	if err != nil {
		return "", false, err
	}
	for {
		gitMarker := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitMarker); err == nil {
			return dir, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func ExpandPath(p string) (string, error) {
	s := strings.TrimSpace(p)
	if s == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(s, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determine home dir: %w", err)
		}
		switch s {
		case "~":
			s = home
		default:
			if strings.HasPrefix(s, "~/") {
				s = filepath.Join(home, s[2:])
			}
		}
	}
	s = os.ExpandEnv(s)
	if !filepath.IsAbs(s) {
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", fmt.Errorf("resolve absolute path: %w", err)
		}
		s = abs
	}
	return filepath.Clean(s), nil
}

func Load(path string) (*Config, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", resolved, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse JSON config %s: %w", resolved, err)
	}
	cfg.Normalize()
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	resolved, err := ResolvePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(resolved, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", resolved, err)
	}
	return nil
}

func Init(path string, force bool) (string, bool, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(resolved); err == nil && !force {
		return resolved, false, fmt.Errorf("config already exists at %s (use --force to overwrite)", resolved)
	}
	cfg := ExampleConfig()
	if err := Save(resolved, cfg); err != nil {
		return resolved, false, err
	}
	return resolved, true, nil
}

func EnsureGitignoreExcludesMgit(configPath string) (bool, error) {
	resolved, err := ResolvePath(configPath)
	if err != nil {
		return false, err
	}
	if filepath.Base(resolved) != "config.json" {
		return false, nil
	}
	cfgDir := filepath.Dir(resolved)
	if filepath.Base(cfgDir) != ".mgit" {
		return false, nil
	}
	repoRoot := filepath.Dir(cfgDir)
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	st, err := os.Stat(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", gitignorePath, err)
	}
	if st.IsDir() {
		return false, nil
	}
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", gitignorePath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case ".mgit", ".mgit/":
			return false, nil
		}
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(".mgit\n")
	if err := os.WriteFile(gitignorePath, []byte(b.String()), st.Mode().Perm()); err != nil {
		return false, fmt.Errorf("write %s: %w", gitignorePath, err)
	}
	return true, nil
}

func ExampleConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Rules: []Rule{
			{ID: "work-github", Host: "github.com", Owner: "CompanyOrg", Key: "~/.ssh/work_key"},
			{ID: "personal-github", Host: "github.com", Owner: "MyUser", Key: "~/.ssh/personal_key"},
			{ID: "default-github", Host: "github.com", Owner: "*", Key: "~/.ssh/default_github_key"},
			{ID: "default", Host: "*", Owner: "*", Key: "~/.ssh/default_key"},
		},
	}
}

func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	for i := range c.Rules {
		r := &c.Rules[i]
		r.Host = normalizePattern(r.Host)
		r.Owner = normalizePattern(r.Owner)
		r.Key = strings.TrimSpace(r.Key)
		if r.ID == "" {
			r.ID = newRuleID()
		}
	}
}

func normalizePattern(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "*"
	}
	return s
}

func (c *Config) AddRule(r Rule, force bool) error {
	c.Normalize()
	r.Host = normalizePattern(r.Host)
	r.Owner = normalizePattern(r.Owner)
	r.Key = strings.TrimSpace(r.Key)
	if r.Key == "" {
		return errors.New("key path is required")
	}
	if r.ID == "" {
		r.ID = newRuleID()
	}
	for _, existing := range c.Rules {
		if strings.EqualFold(existing.Host, r.Host) &&
			strings.EqualFold(existing.Owner, r.Owner) &&
			existing.Key == r.Key &&
			existing.Priority == r.Priority {
			if !force {
				return fmt.Errorf("rule already exists (id=%s); use --force to add duplicate", existing.ID)
			}
		}
	}
	c.Rules = append(c.Rules, r)
	return nil
}

func (c *Config) RemoveRule(sel RemoveSelector) (Rule, bool) {
	c.Normalize()
	if sel.Index > 0 && sel.Index <= len(c.Rules) {
		i := sel.Index - 1
		r := c.Rules[i]
		c.Rules = append(c.Rules[:i], c.Rules[i+1:]...)
		return r, true
	}
	for i, r := range c.Rules {
		if sel.ID != "" && r.ID == sel.ID {
			c.Rules = append(c.Rules[:i], c.Rules[i+1:]...)
			return r, true
		}
		if matchesRemoveSelector(r, sel) {
			c.Rules = append(c.Rules[:i], c.Rules[i+1:]...)
			return r, true
		}
	}
	return Rule{}, false
}

func matchesRemoveSelector(r Rule, sel RemoveSelector) bool {
	if sel.Host == "" && sel.Owner == "" && sel.Key == "" {
		return false
	}
	if sel.Host != "" && !strings.EqualFold(r.Host, sel.Host) {
		return false
	}
	if sel.Owner != "" && !strings.EqualFold(r.Owner, sel.Owner) {
		return false
	}
	if sel.Key != "" && r.Key != sel.Key {
		return false
	}
	return true
}

func (c *Config) Validate() []ValidationIssue {
	c.Normalize()
	var issues []ValidationIssue
	if c.Version <= 0 {
		issues = append(issues, ValidationIssue{Level: "error", Field: "version", Message: "version must be >= 1"})
	}
	seenExact := map[string]string{}
	for i, r := range c.Rules {
		prefix := fmt.Sprintf("rules[%d]", i)
		if strings.TrimSpace(r.Key) == "" {
			issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".key", Message: "key is required"})
		}
		if _, err := validatePattern(r.Host); err != nil {
			issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".host", Message: err.Error()})
		}
		if _, err := validatePattern(r.Owner); err != nil {
			issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".owner", Message: err.Error()})
		}
		if r.Key != "" {
			expanded, err := ExpandPath(r.Key)
			if err != nil {
				issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".key", Message: err.Error()})
			} else if st, statErr := os.Stat(expanded); statErr != nil {
				issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".key", Message: fmt.Sprintf("key file not found: %s", expanded)})
			} else if st.IsDir() {
				issues = append(issues, ValidationIssue{Level: "error", Field: prefix + ".key", Message: fmt.Sprintf("key path is a directory: %s", expanded)})
			}
		}
		key := strings.ToLower(r.Host) + "|" + strings.ToLower(r.Owner) + "|" + fmt.Sprintf("%d", r.Priority)
		if prevID, ok := seenExact[key]; ok {
			issues = append(issues, ValidationIssue{
				Level:   "warning",
				Field:   prefix,
				Message: fmt.Sprintf("possible conflict with rule id=%s (same host/owner/priority)", prevID),
			})
		} else {
			seenExact[key] = r.ID
		}
	}
	return issues
}

func HasErrors(issues []ValidationIssue) bool {
	for _, i := range issues {
		if i.Level == "error" {
			return true
		}
	}
	return false
}

func SortedRulesCopy(rules []Rule) []Rule {
	out := append([]Rule(nil), rules...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func validatePattern(p string) (string, error) {
	p = normalizePattern(p)
	_, err := filepath.Match(p, "example")
	if err != nil {
		return "", fmt.Errorf("invalid wildcard pattern %q: %w", p, err)
	}
	return p, nil
}

func newRuleID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "rule"
	}
	return "r_" + hex.EncodeToString(b[:])
}
