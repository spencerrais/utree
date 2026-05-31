package app

import (
	"bufio"
	"io"
	"strings"
)

func confirmed(reader io.Reader) bool {
	if reader == nil {
		return false
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
