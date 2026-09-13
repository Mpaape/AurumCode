# Integration

- Wire the sandbox: implement `testgen.Runner` to invoke the existing OCI
  sandbox with `go test ./<packagePath>`, passing `dir` as the checkout root,
  and returning `Result{ExitCode, Output}` from the container's exit code and
  captured output.
- Propose: call `testgen.Propose(diff)` and forward each `TestCase.Name` into
  the sandbox's generated `_test.go`.
- Execute: call `testgen.Run(ctx, plan, root, runner)`; it deduplicates and
  sorts packages so each is tested once.
- Report: map each `Outcome` into the review report (`Passed`, `Output`) next
  to the reviewer's findings, and surface non-zero exit codes as failed checks.
