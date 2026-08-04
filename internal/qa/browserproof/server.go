package browserproof

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxLedgerBodyBytes bounds the copies the harness keeps to check driver claims
// against. Past the budget a route keeps no copy and is reported as
// uncorroborated instead of being believed.
const maxLedgerBodyBytes = 16 << 20

// ledgerEntry is what the local server actually served for a route. It is the
// harness's independent record: a driver observation that the ledger does not
// corroborate cannot become proof.
type ledgerEntry struct {
	Route  string
	Status int
	Bytes  int64
	Digest string

	// NeedsScript is the harness's own reading of the served bytes, not the
	// driver's claim about them.
	NeedsScript bool

	// Body is the harness's copy of exactly what it served, kept so the
	// driver's content claims can be checked against it. It never reaches a
	// verdict: BrowserProofResultV1 carries no markup.
	Body []byte
}

// siteServer publishes a built site on the loopback interface only. It reads
// files under root and reaches no network of its own.
type siteServer struct {
	root     string
	listener net.Listener
	server   *http.Server
	baseURL  string

	mu       sync.Mutex
	ledger   map[string]ledgerEntry
	retained int64
}

func startSiteServer(root string) (*siteServer, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("browserproof: resolve artifact directory: %w", err)
	}
	// The root is resolved through its own links once, so every later
	// containment check compares two real paths instead of two spellings.
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("browserproof: resolve artifact directory: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("browserproof: artifact directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("browserproof: artifact path %q is not a directory", root)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("browserproof: bind loopback listener: %w", err)
	}

	site := &siteServer{
		root:     resolved,
		listener: listener,
		baseURL:  "http://" + listener.Addr().String(),
		ledger:   map[string]ledgerEntry{},
	}
	site.server = &http.Server{
		Handler:           site,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() { _ = site.server.Serve(listener) }()

	return site, nil
}

func (s *siteServer) Close() {
	_ = s.server.Close()
}

func (s *siteServer) URL(route string) string {
	return s.baseURL + normalizeRoute(route)
}

func (s *siteServer) lastServed(route string) (ledgerEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.ledger[normalizeRoute(route)]

	return entry, ok
}

func (s *siteServer) record(entry ledgerEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.retained -= int64(len(s.ledger[entry.Route].Body))
	if s.retained+int64(len(entry.Body)) > maxLedgerBodyBytes {
		entry.Body = nil
	}
	s.retained += int64(len(entry.Body))
	if s.retained < 0 {
		s.retained = 0
	}

	s.ledger[entry.Route] = entry
}

func (s *siteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := normalizeRoute(r.URL.Path)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.fail(w, route, http.StatusMethodNotAllowed)
		return
	}

	candidate, ok := s.resolve(route)
	if !ok {
		s.fail(w, route, http.StatusNotFound)
		return
	}

	full, info, ok := s.insideRoot(candidate)
	if !ok {
		s.fail(w, route, http.StatusNotFound)
		return
	}
	if info.IsDir() {
		if full, info, ok = s.insideRoot(filepath.Join(full, "index.html")); !ok || !info.Mode().IsRegular() {
			s.fail(w, route, http.StatusNotFound)
			return
		}
	}
	if info.Size() > MaxPageBytes {
		s.fail(w, route, http.StatusRequestEntityTooLarge)
		return
	}

	body, err := os.ReadFile(full)
	if err != nil {
		s.fail(w, route, http.StatusInternalServerError)
		return
	}

	mediaType := contentType(full)
	sum := sha256.Sum256(body)
	s.record(ledgerEntry{
		Route:       route,
		Status:      http.StatusOK,
		Bytes:       int64(len(body)),
		Digest:      "sha256:" + hex.EncodeToString(sum[:]),
		NeedsScript: strings.HasPrefix(mediaType, "text/html") && hasExecutableScript(tokenizeHTML(string(body))),
		Body:        body,
	})

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func (s *siteServer) fail(w http.ResponseWriter, route string, status int) {
	s.record(ledgerEntry{Route: route, Status: status})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
}

// resolve maps a route to a file under root, refusing anything that escapes it.
func (s *siteServer) resolve(route string) (string, bool) {
	relative := strings.TrimPrefix(route, "/")
	full := filepath.Join(s.root, filepath.FromSlash(relative))

	if !s.contains(full) {
		return "", false
	}

	return full, true
}

// contains reports whether a real path is the artifact root or lies under it.
func (s *siteServer) contains(candidate string) bool {
	return candidate == s.root || strings.HasPrefix(candidate, s.root+string(os.PathSeparator))
}

// insideRoot decides whether one candidate path may be served at all. Route
// resolution alone cannot see an escape that a link performs after the join, so
// the entry is inspected without following it and then resolved and checked
// against the root again.
//
// The entry is inspected with Lstat, so a link is seen as a link and not as
// what it points at: only a plain file or a directory of this artifact may be
// served. A link is refused rather than followed because its target is not part
// of the bytes the verdict is about and artifactDigest does not read it either,
// and everything else is refused because reading a pipe or a device never
// returns.
func (s *siteServer) insideRoot(candidate string) (string, os.FileInfo, bool) {
	info, err := os.Lstat(candidate)
	if err != nil {
		return "", nil, false
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return "", nil, false
	}

	// The parents of the candidate may still be links out of the artifact, so
	// the whole path is resolved and checked before anything is read.
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !s.contains(resolved) {
		return "", nil, false
	}

	return resolved, info, true
}

func normalizeRoute(route string) string {
	if route == "" {
		return "/"
	}
	if cut := strings.IndexAny(route, "?#"); cut >= 0 {
		route = route[:cut]
	}
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}

	return path.Clean(route)
}

// resolveLink turns an href found on a page into a route of this site, or
// reports false for anything that would leave the local artifact.
func resolveLink(from, href string) (string, bool) {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "//") || strings.Contains(trimmed, "://") {
		return "", false
	}
	if cut := strings.Index(trimmed, ":"); cut >= 0 && !strings.Contains(trimmed[:cut], "/") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "/") {
		return normalizeRoute(trimmed), true
	}

	return normalizeRoute(path.Join(path.Dir(normalizeRoute(from)), trimmed)), true
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// artifactDigest fingerprints the built site so a verdict is tied to the exact
// bytes that were served.
func artifactDigest(root string) (string, error) {
	// The root is resolved through its own links first, exactly as the server
	// resolves it before serving anything. Walking an unresolved root would meet
	// the link itself, mark it unread and stop: every artifact reached through a
	// link would then carry the same content-free identity while the server
	// served the real files behind it.
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("browserproof: artifact directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("browserproof: artifact directory: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("browserproof: artifact directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("browserproof: artifact path %q is not a directory", root)
	}
	root = resolved

	var lines []string
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relative, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		name := filepath.ToSlash(relative)

		// Lstat, never Stat: what a link points at lives outside the artifact,
		// so it is named in the digest and never read. The same refusal covers
		// pipes and devices, which no read of ours would come back from.
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() {
			lines = append(lines, name+" "+unreadEntryMark(info.Mode()))
			return nil
		}

		body, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}

		sum := sha256.Sum256(body)
		lines = append(lines, name+" "+hex.EncodeToString(sum[:]))

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("browserproof: digest artifact: %w", err)
	}

	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))

	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// unreadEntryMark stands in for an entry the digest refuses to read. It keeps
// such an entry visible in the artifact identity — an artifact that carries a
// link out of its root is not the same artifact as one that does not — while
// carrying nothing about what the entry points at.
func unreadEntryMark(mode fs.FileMode) string {
	if mode&fs.ModeSymlink != 0 {
		return "unread:symlink"
	}

	return "unread:irregular"
}
