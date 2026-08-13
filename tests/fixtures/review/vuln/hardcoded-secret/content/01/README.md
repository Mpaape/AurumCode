# hardcoded-secret fixture

A tiny synthetic project whose second commit plants two hardcoded, fully
synthetic secrets in two different shapes, so `aurumcode review --base
HEAD~1 --seguranca` (card AUR-442) has real vulnerabilities to find:

- `config/secrets.env`: an unquoted `KEY=VALUE` line, the shape
  `tests/fixtures/repos/git-demo`'s own planted secret uses.
- `src/config.py`: a quoted Python assignment, the shape a hardcoded
  credential more commonly takes in application source.

Nothing here is a real application, credential, or database.
