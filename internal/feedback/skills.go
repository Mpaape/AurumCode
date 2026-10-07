package feedback

import "strings"

// ApplySection replaces the body of "## <section>" in a skill with text, or
// appends the section when the skill does not have it. Every other section
// keeps its bytes.
func ApplySection(content, section, text string) string {
	section = strings.TrimSpace(section)
	lines := strings.Split(content, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") && strings.TrimSpace(strings.TrimPrefix(l, "## ")) == section {
			start = i
			break
		}
	}
	body := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if start < 0 {
		out := strings.TrimRight(content, "\n")
		if out != "" {
			out += "\n\n"
		}
		return out + "## " + section + "\n\n" + strings.Join(body, "\n") + "\n"
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || strings.HasPrefix(lines[i], "# ") {
			end = i
			break
		}
	}
	replaced := append([]string{}, lines[:start+1]...)
	replaced = append(replaced, "")
	replaced = append(replaced, body...)
	if end < len(lines) {
		replaced = append(replaced, "")
		replaced = append(replaced, lines[end:]...)
	} else {
		replaced = append(replaced, "")
	}
	return strings.Join(replaced, "\n")
}
