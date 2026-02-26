package giturl

import "testing"

func TestParseSCPLike(t *testing.T) {
	got, err := Parse("git@github.com:CompanyOrg/project.git")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Host != "github.com" || got.User != "git" || got.Owner != "CompanyOrg" || got.Repo != "project" {
		t.Fatalf("unexpected parsed remote: %+v", got)
	}
	if !got.IsSSH() {
		t.Fatalf("expected SSH transport")
	}
}

func TestParseSSHURLNestedGroup(t *testing.T) {
	got, err := Parse("ssh://git@gitlab.com/Group/subgroup/repo.git")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Host != "gitlab.com" || got.Owner != "Group/subgroup" || got.Repo != "repo" {
		t.Fatalf("unexpected parsed remote: %+v", got)
	}
	if !got.IsSSH() {
		t.Fatalf("expected SSH transport")
	}
}

func TestParseHTTPS(t *testing.T) {
	got, err := Parse("https://github.com/CompanyOrg/project.git")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !got.IsHTTPS() {
		t.Fatalf("expected HTTPS transport")
	}
	if got.Host != "github.com" || got.Owner != "CompanyOrg" || got.Repo != "project" {
		t.Fatalf("unexpected parsed remote: %+v", got)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse("github.com/project"); err == nil {
		t.Fatalf("expected error for invalid input")
	}
}
