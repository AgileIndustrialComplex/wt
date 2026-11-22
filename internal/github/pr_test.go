package github

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestOpenPRsWithParsesGHListOutput(t *testing.T) {
	var gotArgs []string
	run := func(args ...string) (string, error) {
		gotArgs = append([]string{}, args...)
		return `[{"number":17,"title":"Add F key","headRefName":"feature/force-update"},{"number":18,"title":"PR work","headRefName":"feature/pr"}]`, nil
	}

	prs, err := openPRsWith(run)
	if err != nil {
		t.Fatalf("openPRsWith() error = %v", err)
	}
	want := []PR{
		{Number: 17, Title: "Add F key", Branch: "feature/force-update"},
		{Number: 18, Title: "PR work", Branch: "feature/pr"},
	}
	if !reflect.DeepEqual(prs, want) {
		t.Fatalf("openPRsWith() = %+v, want %+v", prs, want)
	}
	wantArgs := []string{"pr", "list", "--state", "open", "--json", "number,title,headRefName"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("gh args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestGetWithParsesGHViewOutput(t *testing.T) {
	var gotArgs []string
	run := func(args ...string) (string, error) {
		gotArgs = append([]string{}, args...)
		return `{"number":123,"title":"Fix thing","headRefName":"fix/thing"}`, nil
	}

	pr, err := getWith(run, "123")
	if err != nil {
		t.Fatalf("getWith() error = %v", err)
	}
	want := PR{Number: 123, Title: "Fix thing", Branch: "fix/thing"}
	if pr != want {
		t.Fatalf("getWith() = %+v, want %+v", pr, want)
	}
	wantArgs := []string{"pr", "view", "123", "--json", "number,title,headRefName"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("gh args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestOpenPRsWithPropagatesGHError(t *testing.T) {
	run := func(args ...string) (string, error) { return "", fmt.Errorf("not logged in") }
	if _, err := openPRsWith(run); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("openPRsWith() error = %v, want gh error propagated", err)
	}
}

func TestGetWithReportsBadJSON(t *testing.T) {
	run := func(args ...string) (string, error) { return "not json", nil }
	if _, err := getWith(run, "1"); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("getWith() error = %v, want parse error", err)
	}
}
