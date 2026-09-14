package changelog

import (
	"strings"
	"testing"
)

// TestAUR498ClassifyCommits covers AC-001: every recognized type, optional
// scope, the trailing bang and the BREAKING CHANGE: footer, plus residual
// subjects that must not be treated as breaking.
func TestAUR498ClassifyCommits(t *testing.T) {
	tests := []struct {
		name         string
		commit       Commit
		wantKind     Kind
		wantScope    string
		wantBreaking bool
		conventional bool
	}{
		{name: "feat", commit: Commit{Subject: "feat: add thing"}, wantKind: KindFeat, conventional: true},
		{name: "feat with scope", commit: Commit{Subject: "feat(api): add thing"}, wantKind: KindFeat, wantScope: "api", conventional: true},
		{name: "feat bang", commit: Commit{Subject: "feat!: drop support"}, wantKind: KindFeat, wantBreaking: true, conventional: true},
		{name: "feat scope bang", commit: Commit{Subject: "feat(api)!: drop support"}, wantKind: KindFeat, wantScope: "api", wantBreaking: true, conventional: true},
		{name: "fix", commit: Commit{Subject: "fix: correct thing"}, wantKind: KindFix, conventional: true},
		{name: "perf", commit: Commit{Subject: "perf: go faster"}, wantKind: KindPerf, conventional: true},
		{name: "refactor", commit: Commit{Subject: "refactor: tidy"}, wantKind: KindRefactor, conventional: true},
		{name: "docs", commit: Commit{Subject: "docs: explain"}, wantKind: KindDocs, conventional: true},
		{name: "chore", commit: Commit{Subject: "chore: bump deps"}, wantKind: KindChore, conventional: true},
		{name: "test", commit: Commit{Subject: "test: add cases"}, wantKind: KindTest, conventional: true},
		{name: "build", commit: Commit{Subject: "build: wire target"}, wantKind: KindBuild, conventional: true},
		{name: "ci", commit: Commit{Subject: "ci: run lint"}, wantKind: KindCI, conventional: true},
		{name: "style", commit: Commit{Subject: "style: format"}, wantKind: KindStyle, conventional: true},
		{
			name:         "breaking footer",
			commit:       Commit{Subject: "feat: change api", Body: "some body\n\nBREAKING CHANGE: removed endpoint\n"},
			wantKind:     KindFeat,
			wantBreaking: true,
			conventional: true,
		},
		{
			name:         "dash breaking footer",
			commit:       Commit{Subject: "fix: change api", Body: "BREAKING-CHANGE: removed endpoint"},
			wantKind:     KindFix,
			wantBreaking: true,
			conventional: true,
		},
		{
			name:         "non conventional subject is residual",
			commit:       Commit{Subject: "just some words"},
			wantKind:     KindUnknown,
			conventional: false,
		},
		{
			name:         "unrecognized type is residual",
			commit:       Commit{Subject: "wip(scope): not conventional here"},
			wantKind:     KindUnknown,
			conventional: false,
		},
		{
			name:         "release-like subject is not breaking",
			commit:       Commit{Subject: "chore: release v9.0.0"},
			wantKind:     KindChore,
			conventional: true,
		},
		{
			name:         "residual with footer is still breaking",
			commit:       Commit{Subject: "not conventional", Body: "BREAKING CHANGE: big"},
			wantKind:     KindUnknown,
			wantBreaking: true,
			conventional: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.commit)
			if got.Kind != tt.wantKind {
				t.Fatalf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Scope != tt.wantScope {
				t.Fatalf("Scope = %q, want %q", got.Scope, tt.wantScope)
			}
			if got.Breaking != tt.wantBreaking {
				t.Fatalf("Breaking = %v, want %v", got.Breaking, tt.wantBreaking)
			}
			if got.Conventional != tt.conventional {
				t.Fatalf("Conventional = %v, want %v", got.Conventional, tt.conventional)
			}
		})
	}
}

// TestAUR498SemverBump covers AC-002: the exact bump table, including the 0.x
// breaking exception, and that the highest applicable rule wins.
func TestAUR498SemverBump(t *testing.T) {
	tests := []struct {
		name    string
		current Version
		commits []Commit
		want    Version
		bump    Bump
	}{
		{name: "breaking majors", current: Version{1, 2, 3}, commits: []Commit{{Subject: "feat!: drop"}}, want: Version{2, 0, 0}, bump: BumpMajor},
		{name: "breaking footer majors", current: Version{1, 2, 3}, commits: []Commit{{Subject: "fix: drop", Body: "BREAKING CHANGE: gone"}}, want: Version{2, 0, 0}, bump: BumpMajor},
		{name: "zero breaking minors", current: Version{0, 4, 2}, commits: []Commit{{Subject: "feat!: drop"}}, want: Version{0, 5, 0}, bump: BumpMinor},
		{name: "feat minors", current: Version{1, 2, 3}, commits: []Commit{{Subject: "feat: add"}}, want: Version{1, 3, 0}, bump: BumpMinor},
		{name: "zero feat minors", current: Version{0, 4, 2}, commits: []Commit{{Subject: "feat: add"}}, want: Version{0, 5, 0}, bump: BumpMinor},
		{name: "fix patches", current: Version{1, 2, 3}, commits: []Commit{{Subject: "fix: boo"}}, want: Version{1, 2, 4}, bump: BumpPatch},
		{name: "perf patches", current: Version{1, 2, 3}, commits: []Commit{{Subject: "perf: zoom"}}, want: Version{1, 2, 4}, bump: BumpPatch},
		{name: "docs and chore do not bump", current: Version{1, 2, 3}, commits: []Commit{{Subject: "docs: x"}, {Subject: "chore: y"}}, want: Version{1, 2, 3}, bump: BumpNone},
		{name: "residual does not bump", current: Version{1, 2, 3}, commits: []Commit{{Subject: "random words"}}, want: Version{1, 2, 3}, bump: BumpNone},
		{name: "feat beats fix", current: Version{1, 2, 3}, commits: []Commit{{Subject: "fix: a"}, {Subject: "feat: b"}}, want: Version{1, 3, 0}, bump: BumpMinor},
		{name: "breaking beats feat", current: Version{1, 2, 3}, commits: []Commit{{Subject: "feat: a"}, {Subject: "fix!: b"}}, want: Version{2, 0, 0}, bump: BumpMajor},
		{name: "release prose does not bump", current: Version{1, 2, 3}, commits: []Commit{{Subject: "chore: release v9.0.0"}}, want: Version{1, 2, 3}, bump: BumpNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bump := NextVersion(tt.current, tt.commits)
			if got != tt.want || bump != tt.bump {
				t.Fatalf("NextVersion = (%s, %s), want (%s, %s)", got, bump, tt.want, tt.bump)
			}
		})
	}
}

// TestAUR498Render covers AC-003: the exact Markdown shape, fixed section
// order, stable bullet order, and byte-identical repeat runs.
func TestAUR498Render(t *testing.T) {
	commits := []Commit{
		{Subject: "feat(api): add endpoint", Hash: "abc1234"},
		{Subject: "perf: speed up parser"},
		{Subject: "refactor: tidy loops"},
		{Subject: "fix: correct off-by-one"},
		{Subject: "docs: update readme"},
		{Subject: "totally non conventional subject"},
		{Subject: "feat: add second thing"},
	}
	const want = "## 1.3.0\n" +
		"\n### Added\n" +
		"\n- feat(api): add endpoint (abc1234)\n" +
		"\n- feat: add second thing\n" +
		"\n### Changed\n" +
		"\n- perf: speed up parser\n" +
		"\n- refactor: tidy loops\n" +
		"\n- totally non conventional subject\n" +
		"\n### Fixed\n" +
		"\n- fix: correct off-by-one\n" +
		"\n### Removed\n"

	got := Render(Version{1, 3, 0}, commits)
	if got != want {
		t.Fatalf("Render mismatch\n got:\n%q\nwant:\n%q", got, want)
	}
	if second := Render(Version{1, 3, 0}, commits); second != got {
		t.Fatalf("Render is not deterministic:\nfirst:  %q\nsecond: %q", got, second)
	}

	added := strings.Index(got, "add endpoint")
	added2 := strings.Index(got, "add second thing")
	perf := strings.Index(got, "speed up parser")
	refactor := strings.Index(got, "tidy loops")
	fix := strings.Index(got, "correct off-by-one")
	if !(added < added2 && added2 < perf && perf < refactor && refactor < fix) {
		t.Fatalf("section order is not stable: %d %d %d %d %d", added, added2, perf, refactor, fix)
	}
}

// TestAUR498UntrustedText covers AC-004: escaping, advisory release prose,
// bounded input, and no panic on nil or empty input.
func TestAUR498UntrustedText(t *testing.T) {
	t.Run("html and markdown escaped", func(t *testing.T) {
		out := Render(Version{1, 0, 1}, []Commit{
			{Subject: "feat: <script>alert(1)</script>"},
			{Subject: "fix: a & b"},
			{Subject: "fix: use `code` and *stars*"},
		})
		if strings.Contains(out, "<script>") {
			t.Fatalf("raw script tag survived rendering:\n%s", out)
		}
		for _, want := range []string{"&lt;script&gt;", "&amp;", "\\`code\\`", "\\*stars\\*"} {
			if !strings.Contains(out, want) {
				t.Fatalf("expected %q in output:\n%s", want, out)
			}
		}
	})

	t.Run("release prose cannot bump", func(t *testing.T) {
		cl := Classify(Commit{Subject: "chore: release v9.0.0"})
		if cl.Breaking {
			t.Fatalf("release prose marked breaking")
		}
		got, bump := NextVersion(Version{1, 2, 3}, []Commit{{Subject: "chore: release v9.0.0"}})
		if bump != BumpNone || got != (Version{1, 2, 3}) {
			t.Fatalf("release prose changed version: (%s, %s)", got, bump)
		}
	})

	t.Run("input count is bounded", func(t *testing.T) {
		commits := make([]Commit, MaxCommits+50)
		for i := range commits {
			commits[i] = Commit{Subject: "fix: bounded"}
		}
		if got := ClassifyCommits(commits); len(got) != MaxCommits {
			t.Fatalf("ClassifyCommits len = %d, want %d", len(got), MaxCommits)
		}
	})

	t.Run("output length is bounded", func(t *testing.T) {
		commits := make([]Commit, MaxCommits)
		long := strings.Repeat("&", MaxSubjectLen*2)
		for i := range commits {
			commits[i] = Commit{Subject: "feat: " + long}
		}
		out := Render(Version{2, 0, 0}, commits)
		if len(out) > MaxOutputLen {
			t.Fatalf("Render produced %d bytes, cap is %d", len(out), MaxOutputLen)
		}
		if !strings.Contains(out, "truncated") {
			t.Fatalf("expected a truncation marker in bounded output")
		}
	})

	t.Run("nil and empty do not panic", func(t *testing.T) {
		if got := ClassifyCommits(nil); len(got) != 0 {
			t.Fatalf("ClassifyCommits(nil) = %d entries, want 0", len(got))
		}
		if v, bump := NextVersion(Version{1, 0, 0}, nil); v != (Version{1, 0, 0}) || bump != BumpNone {
			t.Fatalf("NextVersion(v, nil) = (%s, %s)", v, bump)
		}
		out := Render(Version{1, 0, 0}, nil)
		for _, heading := range []string{"## 1.0.0", "### Added", "### Changed", "### Fixed", "### Removed"} {
			if !strings.Contains(out, heading) {
				t.Fatalf("empty render missing %q:\n%s", heading, out)
			}
		}
	})
}
