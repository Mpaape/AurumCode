package testgen

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestPropose(t *testing.T) {
	tests := []struct {
		name string
		diff *types.Diff
		want []TestCase
	}{
		{
			name: "functions and methods in deterministic order",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{
						Path: "pkg/service.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{
								"+package pkg",
								"+func New() *Service { return nil }",
								"+func (s *Service) Handle(ctx context.Context) error { return nil }",
								"+func (s *Service) Close() {}",
							},
						}},
					},
					{
						Path: "pkg/helper.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{
								"+func Helper(x int) int { return x }",
							},
						}},
					},
				},
			},
			want: []TestCase{
				{Name: "TestHelper", Package: "pkg", Focus: "Helper"},
				{Name: "TestNew", Package: "pkg", Focus: "New"},
				{Name: "TestServiceClose", Package: "pkg", Focus: "Service.Close"},
				{Name: "TestServiceHandle", Package: "pkg", Focus: "Service.Handle"},
			},
		},
		{
			name: "skips non-go and test files",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{
						Path: "pkg/svc_test.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{"+func TestSvc(t *testing.T) {}"},
						}},
					},
					{
						Path: "README.md",
						Hunks: []types.DiffHunk{{
							Lines: []string{"+# func NotGo()"},
						}},
					},
					{
						Path: "pkg/a.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{"+func A() {}"},
						}},
					},
				},
			},
			want: []TestCase{
				{Name: "TestA", Package: "pkg", Focus: "A"},
			},
		},
		{
			name: "file with additions but no function declaration",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{
						Path: "internal/http_server.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{
								"+\t// tweak the default timeout",
								"+\tconst defaultTimeout = 30 * time.Second",
							},
						}},
					},
				},
			},
			want: []TestCase{
				{Name: "TestHttpServer", Package: "internal", Focus: "internal/http_server.go"},
			},
		},
		{
			name: "deduplicates repeated declarations",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{
						Path: "pkg/dup.go",
						Hunks: []types.DiffHunk{
							{Lines: []string{"+func Foo() {}"}},
							{Lines: []string{"+func Foo() {}"}},
						},
					},
				},
			},
			want: []TestCase{
				{Name: "TestFoo", Package: "pkg", Focus: "Foo"},
			},
		},
		{
			name: "method receiver variants",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{
						Path: "pkg/m.go",
						Hunks: []types.DiffHunk{{
							Lines: []string{
								"+func (*Server) Handle() {}",
								"+func (s *Server[T]) Map() {}",
							},
						}},
					},
				},
			},
			want: []TestCase{
				{Name: "TestServerHandle", Package: "pkg", Focus: "Server.Handle"},
				{Name: "TestServerMap", Package: "pkg", Focus: "Server.Map"},
			},
		},
		{
			name: "nil and empty diffs yield empty plans",
			diff: nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Propose(tt.diff)
			if got == nil {
				t.Fatal("Propose returned nil plan")
			}
			if !reflect.DeepEqual(got.Cases, tt.want) {
				t.Fatalf("Propose() cases = %#v, want %#v", got.Cases, tt.want)
			}
		})
	}
}

func TestProposeEmptyDiff(t *testing.T) {
	got := Propose(&types.Diff{})
	if len(got.Cases) != 0 {
		t.Fatalf("Propose(empty) cases = %#v, want none", got.Cases)
	}
}

func TestRun(t *testing.T) {
	t.Run("deduplicates and sorts packages", func(t *testing.T) {
		var calls []string
		runner := func(_ context.Context, _, pkg string) (Result, error) {
			calls = append(calls, pkg)
			return Result{Package: pkg, ExitCode: 0, Output: "ok"}, nil
		}

		plan := &Plan{Cases: []TestCase{
			{Name: "TestB", Package: "b"},
			{Name: "TestA", Package: "a"},
			{Name: "TestB2", Package: "b"},
		}}

		outcomes, err := Run(context.Background(), plan, "/root", runner)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if want := []string{"a", "b"}; !reflect.DeepEqual(calls, want) {
			t.Fatalf("runner called with %v, want %v", calls, want)
		}

		want := []Outcome{
			{Package: "a", Passed: true, Output: "ok"},
			{Package: "b", Passed: true, Output: "ok"},
		}
		if !reflect.DeepEqual(outcomes, want) {
			t.Fatalf("Run() outcomes = %#v, want %#v", outcomes, want)
		}
	})

	t.Run("maps exit code to passed", func(t *testing.T) {
		runner := func(_ context.Context, _, pkg string) (Result, error) {
			code := 0
			if pkg == "failing" {
				code = 1
			}
			return Result{Package: pkg, ExitCode: code, Output: "out"}, nil
		}

		plan := &Plan{Cases: []TestCase{
			{Name: "TestX", Package: "ok"},
			{Name: "TestY", Package: "failing"},
		}}

		outcomes, err := Run(context.Background(), plan, "/root", runner)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if outcomes[0].Passed {
			t.Errorf("outcomes[0].Passed = true, want false")
		}
		if !outcomes[1].Passed {
			t.Errorf("outcomes[1].Passed = false, want true")
		}
	})

	t.Run("propagates runner error", func(t *testing.T) {
		boom := errors.New("sandbox failed")
		runner := func(_ context.Context, _, _ string) (Result, error) {
			return Result{}, boom
		}

		plan := &Plan{Cases: []TestCase{{Name: "TestX", Package: "a"}}}

		if _, err := Run(context.Background(), plan, "/root", runner); !errors.Is(err, boom) {
			t.Fatalf("Run() error = %v, want %v", err, boom)
		}
	})

	t.Run("nil plan and nil runner", func(t *testing.T) {
		if outcomes, err := Run(context.Background(), nil, "/root", nil); err != nil || outcomes != nil {
			t.Fatalf("Run(nil plan) = %v, %v; want nil, nil", outcomes, err)
		}

		plan := &Plan{Cases: []TestCase{{Name: "TestX", Package: "a"}}}
		if _, err := Run(context.Background(), plan, "/root", nil); err == nil {
			t.Fatal("Run(nil runner) error = nil, want error")
		}
	})
}
