package apply

import (
	"fmt"
	"strings"
)

// CreateFilePatch renders the unified diff that creates a new file at name
// holding content: "--- /dev/null", "+++ b/name" and one hunk of additions.
// An unsafe name or empty content yields an empty patch.
func CreateFilePatch(name, content string) string {
	name = normalizePath(name)
	lines := splitLines(content)
	if name == "" || len(lines) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", name, len(lines))
	for _, l := range lines {
		sb.WriteString("+" + l + "\n")
	}
	return sb.String()
}

// DeleteFilePatch renders the unified diff that removes the file at name,
// whose current content is content: "--- a/name", "+++ /dev/null" and one
// hunk of removals (with git's marker when the last line has no newline).
func DeleteFilePatch(name, content string) string {
	name = normalizePath(name)
	if name == "" || content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ /dev/null\n@@ -1,%d +0,0 @@\n", name, len(lines))
	for _, l := range lines {
		sb.WriteString("-" + l + "\n")
	}
	if !strings.HasSuffix(content, "\n") {
		sb.WriteString("\\ No newline at end of file\n")
	}
	return sb.String()
}
