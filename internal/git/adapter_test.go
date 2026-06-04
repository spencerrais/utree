package git

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorktreeListPorcelainParsesRepresentativeEntries(t *testing.T) {
	output := strings.Join([]string{
		"worktree /repo/main",
		"HEAD 1111111111111111111111111111111111111111",
		"branch refs/heads/main",
		"",
		"worktree /repo/feature-a",
		"HEAD 2222222222222222222222222222222222222222",
		"branch refs/heads/feature-a",
		"",
		"worktree /repo/detached",
		"HEAD 3333333333333333333333333333333333333333",
		"detached",
		"",
	}, "\n")

	got, err := ParseWorktreeListPorcelain(output)
	if err != nil {
		t.Fatalf("ParseWorktreeListPorcelain returned error: %v", err)
	}

	want := []Worktree{
		{Path: "/repo/main", Branch: "main", Name: "main"},
		{Path: "/repo/feature-a", Branch: "feature-a", Name: "feature-a"},
		{Path: "/repo/detached", Branch: "", Name: "detached"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected worktrees %#v, got %#v", want, got)
	}
}

func TestGitAdapterConstructsWorktreeListCommand(t *testing.T) {
	runner := &recordingRunner{output: []byte("worktree /repo/main\nbranch refs/heads/main\n")}
	adapter := Adapter{Dir: "/repo/main", Run: runner.run}

	worktrees, err := adapter.WorktreeList()
	if err != nil {
		t.Fatalf("WorktreeList returned error: %v", err)
	}

	assertCommands(t, runner.commands, []recordedCommand{{
		dir:  "/repo/main",
		name: "git",
		args: []string{"worktree", "list", "--porcelain"},
	}})
	if len(worktrees) != 1 || worktrees[0].Name != "main" {
		t.Fatalf("expected parsed main worktree, got %#v", worktrees)
	}
}

func TestGitAdapterConstructsWorktreeAddAndRemoveCommands(t *testing.T) {
	runner := &recordingRunner{}
	adapter := Adapter{Dir: "/repo/main", Run: runner.run}

	if err := adapter.WorktreeAdd("/repo/feature-a", "feature-a"); err != nil {
		t.Fatalf("WorktreeAdd returned error: %v", err)
	}
	if err := adapter.WorktreeAddNewBranch("/repo/bugfix-b", "bugfix-b", "main"); err != nil {
		t.Fatalf("WorktreeAddNewBranch returned error: %v", err)
	}
	if err := adapter.WorktreeRemove("/repo/feature-a"); err != nil {
		t.Fatalf("WorktreeRemove returned error: %v", err)
	}

	assertCommands(t, runner.commands, []recordedCommand{
		{dir: "/repo/main", name: "git", args: []string{"worktree", "add", "/repo/feature-a", "feature-a"}},
		{dir: "/repo/main", name: "git", args: []string{"worktree", "add", "-b", "bugfix-b", "/repo/bugfix-b", "main"}},
		{dir: "/repo/main", name: "git", args: []string{"worktree", "remove", "/repo/feature-a"}},
	})
}

func TestGitAdapterConstructsStatusAndBranchCommands(t *testing.T) {
	runner := &recordingRunner{outputs: [][]byte{
		[]byte(" M README.md\n?? scratch.txt\n"),
		nil,
		nil,
		nil,
	}}
	adapter := Adapter{Dir: "/repo/main", Run: runner.run}

	status, err := adapter.StatusPorcelain("/repo/feature-a")
	if err != nil {
		t.Fatalf("StatusPorcelain returned error: %v", err)
	}
	if status != " M README.md\n?? scratch.txt\n" {
		t.Fatalf("expected status output, got %q", status)
	}

	exists, err := adapter.LocalBranchExists("feature-a")
	if err != nil {
		t.Fatalf("LocalBranchExists returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected branch to exist")
	}

	if err := adapter.DeleteLocalBranch("feature-a"); err != nil {
		t.Fatalf("DeleteLocalBranch returned error: %v", err)
	}
	if err := adapter.ForceDeleteLocalBranch("feature-a"); err != nil {
		t.Fatalf("ForceDeleteLocalBranch returned error: %v", err)
	}

	assertCommands(t, runner.commands, []recordedCommand{
		{dir: "/repo/feature-a", name: "git", args: []string{"status", "--porcelain"}},
		{dir: "/repo/main", name: "git", args: []string{"show-ref", "--verify", "--quiet", "refs/heads/feature-a"}},
		{dir: "/repo/main", name: "git", args: []string{"branch", "-d", "feature-a"}},
		{dir: "/repo/main", name: "git", args: []string{"branch", "-D", "feature-a"}},
	})
	assertNoNetworkCommands(t, runner.commands)
	assertNoRemoteBranchDeletion(t, runner.commands)
}

func TestGitAdapterConstructsMergedBranchCheckCommand(t *testing.T) {
	runner := &recordingRunner{output: []byte("  feature-a\n")}
	adapter := Adapter{Dir: "/repo/main", Run: runner.run}

	merged, err := adapter.BranchMerged("main", "feature-a")
	if err != nil {
		t.Fatalf("BranchMerged returned error: %v", err)
	}
	if !merged {
		t.Fatal("expected branch to be merged")
	}

	assertCommands(t, runner.commands, []recordedCommand{{
		dir:  "/repo/main",
		name: "git",
		args: []string{"branch", "--merged", "main", "--list", "feature-a"},
	}})
	assertNoNetworkCommands(t, runner.commands)
}

func TestGitAdapterReportsMissingLocalBranchWithoutError(t *testing.T) {
	runner := &recordingRunner{err: errors.New("exit status 1")}
	adapter := Adapter{Dir: "/repo/main", Run: runner.run}

	exists, err := adapter.LocalBranchExists("missing")
	if err != nil {
		t.Fatalf("LocalBranchExists returned error: %v", err)
	}
	if exists {
		t.Fatal("expected branch not to exist")
	}
}

type recordedCommand struct {
	dir  string
	name string
	args []string
}

type recordingRunner struct {
	commands []recordedCommand
	output   []byte
	outputs  [][]byte
	err      error
}

func (r *recordingRunner) run(dir string, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{dir: dir, name: name, args: append([]string(nil), args...)})
	if r.err != nil {
		return nil, r.err
	}
	if len(r.outputs) > 0 {
		output := r.outputs[0]
		r.outputs = r.outputs[1:]
		return output, nil
	}
	return r.output, nil
}

func assertCommands(t *testing.T, got []recordedCommand, want []recordedCommand) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected commands %#v, got %#v", want, got)
	}
}

func assertNoNetworkCommands(t *testing.T, commands []recordedCommand) {
	t.Helper()

	joined := commandsString(commands)
	for _, forbidden := range []string{"remote show", "fetch", "pull"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("expected no %q command in %q", forbidden, joined)
		}
	}
}

func assertNoRemoteBranchDeletion(t *testing.T, commands []recordedCommand) {
	t.Helper()

	joined := commandsString(commands)
	if strings.Contains(joined, "push") || strings.Contains(joined, "--delete") {
		t.Fatalf("expected no remote branch deletion command in %q", joined)
	}
}

func commandsString(commands []recordedCommand) string {
	parts := make([]string, 0, len(commands))
	for _, command := range commands {
		parts = append(parts, command.name+" "+strings.Join(command.args, " "))
	}
	return strings.Join(parts, "\n")
}
