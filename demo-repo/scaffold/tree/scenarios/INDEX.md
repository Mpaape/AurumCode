# Scenario catalog

| id | kind | what a review of it should say |
|---|---|---|
| `clean-change` | clean | nothing to report |
| `docs-drift` | documentation | the document no longer matches the code |
| `flaky-test` | test quality | the test depends on wall-clock timing |
| `null-deref` | correctness | an absent lookup result is dereferenced |
| `secret-leak` | security | a placeholder credential sits in source |
| `sql-injection` | security | a query is built by string concatenation |

Six scenarios, listed in the same order the scaffold allowlist seals them.
