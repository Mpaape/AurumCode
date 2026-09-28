# Render integration

`internal/render` turns a local review into a deterministic report:

1. `render.Summary(result, "en")` (or `"pt-BR"`) gives a compact CLI summary.
2. `render.Mermaid(diff)` gives an optional local diagram inferred from changed
   files. It is not proof of runtime flow.

The GitHub PR publication uses one code-review body with findings, evidence,
suggestions and limitations. It does not prepend the CLI summary or append the
inferred diagram.
