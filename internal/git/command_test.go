package git

import "testing"

func TestCleanLocalEnvRemovesRepositoryScopedGitVariables(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"GIT_DIR=/repo/.git",
		"GIT_WORK_TREE=/repo",
		"GIT_INDEX_FILE=/repo/.git/index",
		"GIT_AUTHOR_NAME=Test Author",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}

	cleaned := CleanLocalEnv(env)
	got := map[string]bool{}
	for _, entry := range cleaned {
		got[entry] = true
	}

	for _, removed := range []string{"GIT_DIR=/repo/.git", "GIT_WORK_TREE=/repo", "GIT_INDEX_FILE=/repo/.git/index"} {
		if got[removed] {
			t.Fatalf("expected %s to be removed, got %v", removed, cleaned)
		}
	}
	for _, kept := range []string{"PATH=/usr/bin", "GIT_AUTHOR_NAME=Test Author", "GIT_COMMITTER_EMAIL=test@example.com"} {
		if !got[kept] {
			t.Fatalf("expected %s to be kept, got %v", kept, cleaned)
		}
	}
}
