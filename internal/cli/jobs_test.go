package cli

import (
	"strings"
	"testing"
)

func TestJobsCmd(t *testing.T) {
	dirs := []string{"--data-dir", seedCollection(t, "somerville")}
	if _, err := run(t, append([]string{"jobs", "run", "missing"}, dirs...)...); err == nil {
		t.Error("an unknown collection should be rejected")
	}
	for range 2 {
		if out, err := run(t, append([]string{"jobs", "run", "somerville"}, dirs...)...); err != nil || out != "job 1 queued\n" {
			t.Errorf("enqueue: %v %q", err, out)
		}
	}
	if out, err := run(t, append([]string{"jobs"}, dirs...)...); err != nil || !strings.Contains(out, "1   run_collection  somerville  manual   queued") {
		t.Errorf("list: %v\n%s", err, out)
	}
	if out, err := run(t, append([]string{"jobs", "show", "1"}, dirs...)...); err != nil || !strings.Contains(out, `params: {"collection_id":"somerville"}`) {
		t.Errorf("show: %v\n%s", err, out)
	}
}
