package cli

import (
	"strings"
	"testing"
)

func TestAPIKeys(t *testing.T) {
	data := t.TempDir()
	out, err := run(t, "apikeys", "create", "agent", "--data-dir", data)
	token := strings.TrimSpace(out)
	if err != nil || !strings.HasPrefix(token, "hk_") || strings.Contains(token, "\n") {
		t.Fatalf("create: %v %q", err, out)
	}
	out, err = run(t, "apikeys", "--data-dir", data)
	if err != nil || !strings.Contains(out, "agent") || !strings.Contains(out, token[:9]+"…") || strings.Contains(out, token) || !strings.Contains(out, "never") {
		t.Errorf("list: %v\n%s", err, out)
	}
	if _, err := run(t, "apikeys", "create", " ", "--data-dir", data); err == nil {
		t.Error("blank name should be rejected")
	}
	if out, err := run(t, "apikeys", "delete", "1", "--data-dir", data); err != nil || !strings.Contains(out, "key 1 deleted") {
		t.Errorf("delete: %v %q", err, out)
	}
	if _, err := run(t, "apikeys", "delete", "1", "--data-dir", data); err == nil {
		t.Error("deleting a missing key should fail")
	}
}
