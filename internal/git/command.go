package git

import (
	"os"
	"os/exec"
	"strings"
)

var localEnvVars = map[string]struct{}{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
	"GIT_COMMON_DIR":                   {},
	"GIT_CONFIG":                       {},
	"GIT_CONFIG_COUNT":                 {},
	"GIT_CONFIG_PARAMETERS":            {},
	"GIT_DIR":                          {},
	"GIT_GRAFT_FILE":                   {},
	"GIT_IMPLICIT_WORK_TREE":           {},
	"GIT_INDEX_FILE":                   {},
	"GIT_NO_REPLACE_OBJECTS":           {},
	"GIT_OBJECT_DIRECTORY":             {},
	"GIT_PREFIX":                       {},
	"GIT_REPLACE_REF_BASE":             {},
	"GIT_SHALLOW_FILE":                 {},
	"GIT_WORK_TREE":                    {},
}

func Command(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = CleanLocalEnv(os.Environ())
	return cmd
}

func CleanLocalEnv(env []string) []string {
	cleaned := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		if _, remove := localEnvVars[name]; remove {
			continue
		}
		cleaned = append(cleaned, entry)
	}
	return cleaned
}
