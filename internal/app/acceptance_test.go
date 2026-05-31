package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceUnsupportedCommandsAndOptionsAreClear(t *testing.T) {
	testCases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "list all", args: []string{"list", "--all"}, wantStderr: "usage: ut list"},
		{name: "doctor", args: []string{"doctor"}, wantStderr: "unknown command: doctor"},
		{name: "config init", args: []string{"config", "init"}, wantStderr: "usage: ut config info"},
		{name: "remove yes", args: []string{"remove", "feature-a", "--yes"}, wantStderr: "usage: ut remove"},
		{name: "unknown", args: []string{"bogus"}, wantStderr: "unknown command: bogus"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run(testCase.args)

			if exitCode == 0 {
				t.Fatal("expected non-zero exit code")
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			assertContains(t, stderr.String(), testCase.wantStderr)
		})
	}
}

func TestAcceptanceRepositoryHasReadmeAndMITLicense(t *testing.T) {
	repoRoot := appRepoRoot(t)

	readme := readAcceptanceFile(t, filepath.Join(repoRoot, "README.md"))
	for _, command := range []string{"adopt", "convert", "new", "open", "list", "remove", "config info", "info"} {
		assertContains(t, readme, "ut "+command)
	}
	assertContains(t, readme, "go build -o ~/.local/bin/ut ./cmd/ut")
	assertContains(t, readme, "export PATH=\"$HOME/.local/bin:$PATH\"")
	assertContains(t, strings.ToLower(readme), "unsupported")

	license := readAcceptanceFile(t, filepath.Join(repoRoot, "LICENSE"))
	assertContains(t, license, "MIT License")
	assertContains(t, license, "Permission is hereby granted")
}

func appRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readAcceptanceFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
