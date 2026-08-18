package cao

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkDirExplicit(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveWorkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir && got.Path != mustAbs(dir) {
		// Abs may clean the path
		abs, _ := filepath.Abs(dir)
		if got.Path != abs {
			t.Fatalf("path %q want %q", got.Path, abs)
		}
	}
	if got.Reason != "explicit --dir" {
		t.Fatalf("reason %q", got.Reason)
	}
}

func TestResolveWorkDirSkipsAgentpickCheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "company.yaml"), []byte("name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "commands", "start.md"), []byte("# start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	iso := filepath.Join(root, "tmp", "cursor-chat", "git_repos", "workspace")
	if err := os.MkdirAll(filepath.Join(iso, ".cursor", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iso, ".cursor", "commands", "start.md"), []byte("# start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ap := filepath.Join(root, "tmp", "cursor-chat", "git_repos", "agentpick")
	if err := os.MkdirAll(ap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ap, "go.mod"), []byte("module github.com/marko-durasic/agentpick\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTPICK_CAO_WORKDIR", "")
	t.Setenv("DUREEF_AGENT_CLONE_SLUG", "cursor-chat")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ap); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	got, err := ResolveWorkDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != iso {
		t.Fatalf("got %q want isolated %q", got.Path, iso)
	}
	if !got.HasCLI {
		t.Fatal("expected slash commands")
	}
}

func mustAbs(p string) string {
	a, _ := filepath.Abs(p)
	return a
}
