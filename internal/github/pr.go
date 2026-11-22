// Package github reads pull requests through the gh CLI. It never
// reimplements GitHub's API or authentication.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PR is the minimum information wt needs to switch to a pull request.
type PR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Branch string `json:"headRefName"`
}

type runnerFunc func(args ...string) (string, error)

func runGH(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// OpenPRs lists the open pull requests of the repository in the current
// directory, newest first as returned by gh.
func OpenPRs() ([]PR, error) {
	return openPRsWith(runGH)
}

func openPRsWith(run runnerFunc) ([]PR, error) {
	out, err := run("pr", "list", "--state", "open", "--json", "number,title,headRefName")
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	return prs, nil
}

// Get returns the pull request identified by a number or URL.
func Get(number string) (PR, error) {
	return getWith(runGH, number)
}

func getWith(run runnerFunc, number string) (PR, error) {
	out, err := run("pr", "view", number, "--json", "number,title,headRefName")
	if err != nil {
		return PR{}, err
	}
	var pr PR
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return PR{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	return pr, nil
}
