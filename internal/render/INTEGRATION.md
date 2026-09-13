# Render integration

`internal/render` turns a review into PR-ready Markdown. Wire it in two spots:

1. **PR body** — call `render.Summary(result, "en")` (or `"pt-BR"`) and prepend
   the returned Markdown before the detailed findings.
2. **PR comment** — call `render.Mermaid(diff)` and append it inside a fenced
   code block (```` ```mermaid ````) so the changed flow is drawn from the diff.
