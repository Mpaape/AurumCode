package rounds

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Convergence keeps the rounds of one pull request from asking for one more
// thing each time: a later round's new blocking finding on code the earlier
// round already reviewed, unchanged since, is published as an observation,
// not a new block. What the earlier round blocked on still blocks, a changed
// line is judged as usual, and a security finding (Exempt) always blocks.
// Without a trusted earlier round every finding is judged as usual.

// roundMarkerPattern is the one shape of the hidden line a parecer carries
// for the next round: the head commit the round reviewed and the
// fingerprints of the findings it blocked on.
var roundMarkerPattern = regexp.MustCompile(`<!-- aurumcode:round ([0-9a-f]{40}) ([0-9a-f,]*)(?: locs=([A-Za-z0-9_.%+~:,*-]*))? -->`)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Round is what an earlier conclusive round recorded: the head commit it
// reviewed and the fingerprints of its blocking findings.
type Round struct {
	SHA      string
	Blocking []string
	// Locations are where the earlier round's blocking findings were
	// (path and line, without the rule): a finding near one of them may be
	// the same problem renamed or re-anchored, so it is never demoted.
	Locations []Location
	// HasLocations is false for a record written before locations were
	// recorded: then nothing is demoted.
	HasLocations bool
}

// Location is a path and a new-side line.
type Location struct {
	Path string
	Line int
}

// NearDistance is how many lines around an earlier blocking location a
// finding counts as possibly the same problem.
const NearDistance = 5

// RoundMarker is the hidden line recording round; empty when the commit is
// not a full lowercase SHA (nothing the next round could compare against).
// Malformed fingerprints are left out.
func RoundMarker(round Round) string {
	sha := strings.ToLower(strings.TrimSpace(round.SHA))
	if !shaPattern.MatchString(sha) {
		return ""
	}
	seen := map[string]bool{}
	fps := make([]string, 0, len(round.Blocking))
	for _, fp := range round.Blocking {
		if fingerprintPattern.MatchString(fp) && !seen[fp] {
			seen[fp] = true
			fps = append(fps, fp)
		}
	}
	sort.Strings(fps)
	locs := make([]string, 0, len(round.Locations))
	for _, l := range round.Locations {
		if strings.TrimSpace(l.Path) == "" {
			continue
		}
		locs = append(locs, url.QueryEscape(l.Path)+":"+strconv.Itoa(l.Line))
	}
	sort.Strings(locs)
	return "<!-- aurumcode:round " + sha + " " + strings.Join(fps, ",") + " locs=" + strings.Join(locs, ",") + " -->"
}

// ParseRoundMarker reads the last round marker of body.
func ParseRoundMarker(body string) (Round, bool) {
	all := roundMarkerPattern.FindAllStringSubmatch(body, -1)
	if len(all) == 0 {
		return Round{}, false
	}
	m := all[len(all)-1]
	round := Round{SHA: m[1]}
	for _, fp := range strings.Split(m[2], ",") {
		if fingerprintPattern.MatchString(fp) {
			round.Blocking = append(round.Blocking, fp)
		}
	}
	if !strings.Contains(m[0], " locs=") {
		return round, true
	}
	round.HasLocations = true
	for _, item := range strings.Split(m[3], ",") {
		i := strings.LastIndex(item, ":")
		if i <= 0 {
			continue
		}
		path, err := url.QueryUnescape(item[:i])
		line, lerr := strconv.Atoi(item[i+1:])
		if err != nil || lerr != nil {
			// An unreadable location could hide an earlier block.
			round.HasLocations = false
			return round, true
		}
		round.Locations = append(round.Locations, Location{Path: path, Line: line})
	}
	return round, true
}

// MaxComparedFiles is the file count at which the host's comparison may be
// truncated: a comparison that long is not trusted to name every change.
const MaxComparedFiles = 300

// ChangedFile is one file the comparison between the earlier round's
// commit and the current head reports, with its unified patch.
type ChangedFile struct {
	Path  string
	Patch string
}

// Changes is what changed since the earlier round: for each changed file,
// the new-side lines added or modified, or every line (whole) when the
// patch is not available.
type Changes struct {
	lines map[string]map[int]bool
	whole map[string]bool
}

// NewChanges builds the changes from the host's comparison. ok is false
// when the comparison cannot be trusted as "everything that changed since":
// the earlier commit is not an ancestor of the head (force push, rewrite),
// or the file list may be truncated.
func NewChanges(status string, files []ChangedFile) (Changes, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ahead", "identical":
	default:
		return Changes{}, false
	}
	if len(files) >= MaxComparedFiles {
		return Changes{}, false
	}
	c := Changes{lines: map[string]map[int]bool{}, whole: map[string]bool{}}
	for _, f := range files {
		added, ok := addedLines(f.Patch)
		if !ok {
			c.whole[f.Path] = true
			continue
		}
		c.lines[f.Path] = added
	}
	return c, true
}

// LineChanged reports whether the new-side line of path changed since the
// earlier round. A line 0 (a whole-file finding) counts as changed whenever
// the file changed.
func (c Changes) LineChanged(path string, line int) bool {
	if c.whole[path] {
		return true
	}
	added, ok := c.lines[path]
	if !ok {
		return false
	}
	if line <= 0 {
		return true
	}
	return added[line]
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// addedLines reads the new-side numbers of the added lines of a unified
// patch. ok is false for an empty or unreadable patch.
func addedLines(patch string) (map[int]bool, bool) {
	if strings.TrimSpace(patch) == "" {
		return nil, false
	}
	added := map[int]bool{}
	next, inHunk := 0, false
	for _, line := range strings.Split(patch, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, false
			}
			next, inHunk = n, true
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			added[next] = true
			next++
		case strings.HasPrefix(line, "-"):
			// A pure removal changes the code around it: the new lines
			// next to the removed block count as changed.
			added[next-1], added[next], added[next+1] = true, true, true
		case strings.HasPrefix(line, `\`):
		default:
			next++
		}
	}
	return added, inHunk
}

// Finding is one of this round's findings, as convergence reads it.
type Finding struct {
	Fingerprint string
	Path        string
	Line        int
	// Left marks a finding on the old side of the reviewed diff: its line
	// number is not the head's, so it is judged as changed.
	Left bool
	// Blocking is whether the finding would block on its own.
	Blocking bool
	// Exempt marks a finding that always blocks (security, policy,
	// critical).
	Exempt bool
	// Refs are the other places the finding cites (a grouped finding
	// names them in its evidence): if any changed, it is not late.
	Refs []Location
}

// refPattern is a "path:line" reference inside a finding's text.
var refPattern = regexp.MustCompile(`([A-Za-z0-9_][A-Za-z0-9_./-]*):(\d+)`)

// Refs extracts every "path:line" reference of text.
func Refs(text string) []Location {
	var out []Location
	for _, m := range refPattern.FindAllStringSubmatch(text, -1) {
		line, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		out = append(out, Location{Path: m[1], Line: line})
	}
	return out
}

// LateFindings reports, parallel to findings, which blocking findings are
// new in this round on code the earlier round already reviewed and that has
// not changed since: those are published as observations. earlier is the
// set of fingerprints the earlier rounds blocked on or commented. In doubt
// a finding stays as it is: an earlier record without locations demotes
// nothing.
func LateFindings(findings []Finding, earlier map[string]bool, round Round, changes Changes) []bool {
	late := make([]bool, len(findings))
	if !round.HasLocations {
		return late
	}
	for i, f := range findings {
		switch {
		case !f.Blocking, f.Exempt, f.Left, f.Line <= 0:
		case f.Fingerprint == "" || earlier[f.Fingerprint]:
		case nearEarlier(f.Path, f.Line, round.Locations):
		case changes.LineChanged(f.Path, f.Line):
		case changes.anyRefChanged(f.Refs, round.Locations):
		default:
			late[i] = true
		}
	}
	return late
}

// nearEarlier reports a line within NearDistance of an earlier blocking
// location of the same file.
func nearEarlier(path string, line int, locations []Location) bool {
	for _, l := range locations {
		if l.Path != path {
			continue
		}
		if l.Line <= 0 || line <= 0 || abs(l.Line-line) <= NearDistance {
			return true
		}
	}
	return false
}

// anyRefChanged reports a cited place that changed, or sits next to an
// earlier block. A reference naming a file by a suffix of its path counts
// for that file.
func (c Changes) anyRefChanged(refs []Location, locations []Location) bool {
	for _, ref := range refs {
		for _, path := range c.pathsFor(ref.Path) {
			if c.LineChanged(path, ref.Line) {
				return true
			}
		}
		for _, l := range locations {
			if (l.Path == ref.Path || strings.HasSuffix(l.Path, "/"+ref.Path)) && abs(l.Line-ref.Line) <= NearDistance {
				return true
			}
		}
	}
	return false
}

// pathsFor lists the changed files ref names: the same path, or a path
// ending in "/"+ref.
func (c Changes) pathsFor(ref string) []string {
	var out []string
	match := func(path string) {
		if path == ref || strings.HasSuffix(path, "/"+ref) {
			out = append(out, path)
		}
	}
	for path := range c.lines {
		match(path)
	}
	for path := range c.whole {
		match(path)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
