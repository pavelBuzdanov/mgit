package config

import (
	"os"
	"path/filepath"
	"testing"
)

func canonicalPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	if realDir, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(realDir, base)
	}
	return filepath.Clean(p)
}

func TestValidateConfigOK(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id_test")
	if err := os.WriteFile(key, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	cfg := &Config{
		Version: 1,
		Rules: []Rule{
			{ID: "a", Host: "github.com", Owner: "CompanyOrg", Key: key},
		},
	}
	issues := cfg.Validate()
	if HasErrors(issues) {
		t.Fatalf("expected valid config, got issues: %+v", issues)
	}
}

func TestValidateConfigMissingKeyFile(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Rules: []Rule{
			{ID: "a", Host: "github.com", Owner: "CompanyOrg", Key: "/definitely/missing/key"},
		},
	}
	issues := cfg.Validate()
	if !HasErrors(issues) {
		t.Fatalf("expected validation error, got %+v", issues)
	}
}

func TestValidateConfigDuplicateRulesWarns(t *testing.T) {
	dir := t.TempDir()
	key1 := filepath.Join(dir, "k1")
	key2 := filepath.Join(dir, "k2")
	_ = os.WriteFile(key1, []byte("1"), 0o600)
	_ = os.WriteFile(key2, []byte("2"), 0o600)
	cfg := &Config{
		Version: 1,
		Rules: []Rule{
			{ID: "a", Host: "github.com", Owner: "CompanyOrg", Key: key1},
			{ID: "b", Host: "github.com", Owner: "CompanyOrg", Key: key2},
		},
	}
	issues := cfg.Validate()
	foundWarning := false
	for _, issue := range issues {
		if issue.Level == "warning" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected duplicate warning, got %+v", issues)
	}
}

func TestAddRuleRejectsDuplicateWithoutForce(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Rules: []Rule{
			{ID: "a", Host: "github.com", Owner: "CompanyOrg", Key: "/tmp/key"},
		},
	}
	err := cfg.AddRule(Rule{Host: "github.com", Owner: "CompanyOrg", Key: "/tmp/key"}, false)
	if err == nil {
		t.Fatalf("expected duplicate rejection")
	}
}

func TestResolvePathPrefersRepoLocalConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}
	cfgPath := filepath.Join(repo, ".mgit", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"rules":[]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	subdir := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}

	oldWD, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath(): %v", err)
	}
	if canonicalPath(got) != canonicalPath(cfgPath) {
		t.Fatalf("expected %s, got %s", cfgPath, got)
	}
}

func TestResolvePathDefaultsToRepoRootWhenConfigMissing(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	subdir := filepath.Join(repo, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}

	oldWD, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath(): %v", err)
	}
	want := filepath.Join(repo, ".mgit", "config.json")
	if canonicalPath(got) != canonicalPath(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestEnsureGitignoreExcludesMgitAddsEntry(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}
	gitignore := filepath.Join(repo, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	cfgPath := filepath.Join(repo, ".mgit", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"rules":[]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	changed, err := EnsureGitignoreExcludesMgit(cfgPath)
	if err != nil {
		t.Fatalf("EnsureGitignoreExcludesMgit(): %v", err)
	}
	if !changed {
		t.Fatalf("expected change=true")
	}
	data, _ := os.ReadFile(gitignore)
	if got := string(data); got != "node_modules/\n.mgit\n" {
		t.Fatalf("unexpected .gitignore contents: %q", got)
	}
}

func TestEnsureGitignoreExcludesMgitNoDuplicate(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}
	gitignore := filepath.Join(repo, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(".mgit/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	cfgPath := filepath.Join(repo, ".mgit", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"rules":[]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	changed, err := EnsureGitignoreExcludesMgit(cfgPath)
	if err != nil {
		t.Fatalf("EnsureGitignoreExcludesMgit(): %v", err)
	}
	if changed {
		t.Fatalf("expected no change")
	}
	data, _ := os.ReadFile(gitignore)
	if got := string(data); got != ".mgit/\n" {
		t.Fatalf("unexpected .gitignore contents: %q", got)
	}
}

func TestEnsureGitignoreExcludesMgitNoGitignore(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".mgit"), 0o755); err != nil {
		t.Fatalf("mkdir .mgit: %v", err)
	}
	cfgPath := filepath.Join(repo, ".mgit", "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1,"rules":[]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	changed, err := EnsureGitignoreExcludesMgit(cfgPath)
	if err != nil {
		t.Fatalf("EnsureGitignoreExcludesMgit(): %v", err)
	}
	if changed {
		t.Fatalf("expected no change without .gitignore")
	}
}
