package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	toolversion "github.com/Mpaape/AurumCode/internal/version"
)

// writeAudit writes rec and returns the bytes on disk.
func writeAudit(t *testing.T, rec AuditRecord) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := WriteAuditRecord(path, rec, redaction.NewFilter()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// AUR-611 AC-003: the audit record names the AurumCode that wrote it only
// when the build stamped a version; a dev build writes the same bytes as a
// record never stamped at all, with no tool_version key.
func TestAUR611AuditRecordsToolVersionOnlyWhenNotDev(t *testing.T) {
	base := BuildAuditRecord("digest", "", "o/r", "sha", "model", "comment",
		AuditGate{Decision: "pass"}, nil, nil, AuditCoverage{Complete: true})
	unstamped := writeAudit(t, base)

	for _, stamped := range []string{"v9.9.9", "0123456789ab"} {
		rec := base
		rec.StampToolVersion(toolversion.New(stamped))
		var got map[string]any
		if err := json.Unmarshal(writeAudit(t, rec), &got); err != nil {
			t.Fatal(err)
		}
		if got["tool_version"] != stamped {
			t.Fatalf("stamped %q: tool_version = %v, want %q", stamped, got["tool_version"], stamped)
		}
	}

	for _, dev := range []string{"dev", ""} {
		rec := base
		rec.StampToolVersion(toolversion.New("v9.9.9"))
		rec.StampToolVersion(toolversion.New(dev))
		data := writeAudit(t, rec)
		if string(data) != string(unstamped) {
			t.Fatalf("dev build %q changed the record:\n%s\nwant:\n%s", dev, data, unstamped)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["tool_version"]; ok {
			t.Fatalf("dev build %q wrote tool_version: %s", dev, data)
		}
	}
}
