package runner

import "testing"

func TestInferGitTargetPushRemote(t *testing.T) {
	got, err := InferGitTarget([]string{"push", "origin", "main"})
	if err != nil {
		t.Fatalf("InferGitTarget() error = %v", err)
	}
	if got.Kind != TargetRemote || got.RemoteName != "origin" {
		t.Fatalf("unexpected target: %+v", got)
	}
}

func TestInferGitTargetCloneURL(t *testing.T) {
	got, err := InferGitTarget([]string{"clone", "git@github.com:CompanyOrg/project.git"})
	if err != nil {
		t.Fatalf("InferGitTarget() error = %v", err)
	}
	if got.Kind != TargetURL {
		t.Fatalf("expected URL target, got %+v", got)
	}
}

func TestInferGitTargetPullNoExplicitRemote(t *testing.T) {
	got, err := InferGitTarget([]string{"pull"})
	if err != nil {
		t.Fatalf("InferGitTarget() error = %v", err)
	}
	if got.Kind != TargetNone || got.Command != "pull" {
		t.Fatalf("unexpected target: %+v", got)
	}
}

func TestInferGitTargetSkipsFlags(t *testing.T) {
	got, err := InferGitTarget([]string{"fetch", "--prune", "mirror"})
	if err != nil {
		t.Fatalf("InferGitTarget() error = %v", err)
	}
	if got.Kind != TargetRemote || got.RemoteName != "mirror" {
		t.Fatalf("unexpected target: %+v", got)
	}
}

func TestInferGitTargetLSRemoteByURL(t *testing.T) {
	got, err := InferGitTarget([]string{"ls-remote", "ssh://git@gitlab.com/Group/repo.git"})
	if err != nil {
		t.Fatalf("InferGitTarget() error = %v", err)
	}
	if got.Kind != TargetURL {
		t.Fatalf("expected URL target, got %+v", got)
	}
}
