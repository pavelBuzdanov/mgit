package matcher

import (
	"testing"

	"mgit/internal/config"
	"mgit/internal/giturl"
)

func mustParse(t *testing.T, s string) *giturl.ParsedRemote {
	t.Helper()
	p, err := giturl.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return p
}

func TestMatchPrefersSpecificOwnerOverWildcard(t *testing.T) {
	parsed := mustParse(t, "git@github.com:CompanyOrg/proj.git")
	rules := []config.Rule{
		{ID: "wild", Host: "github.com", Owner: "*", Key: "/k/default"},
		{ID: "spec", Host: "github.com", Owner: "CompanyOrg", Key: "/k/work"},
	}
	got, err := Match(rules, parsed)
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.Rule.ID != "spec" {
		t.Fatalf("expected specific rule, got %s", got.Rule.ID)
	}
}

func TestMatchSupportsDefaultFallback(t *testing.T) {
	parsed := mustParse(t, "git@gitlab.com:AnotherOrg/repo.git")
	rules := []config.Rule{
		{ID: "default", Host: "*", Owner: "*", Key: "/k/default"},
	}
	got, err := Match(rules, parsed)
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.Rule.ID != "default" {
		t.Fatalf("unexpected rule: %+v", got.Rule)
	}
}

func TestMatchNoRule(t *testing.T) {
	parsed := mustParse(t, "git@github.com:CompanyOrg/proj.git")
	if _, err := Match(nil, parsed); err == nil {
		t.Fatalf("expected no-match error")
	}
}
