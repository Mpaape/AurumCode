# AurumCode consumer demo repository

This checkout is produced by `demo-repo/scaffold`. Everything here is synthetic
demo material written for the demo office (`O09-demo`): it carries review
scenarios and nothing else.

No file in this checkout comes from the AurumCode repository itself. The
bootstrap that produced it copies only the paths sealed in the scaffold
allowlist, refuses any path that is not on that allowlist, and refuses any
payload that carries a private AurumCode marker or a credential shape.

## Layout

- `CONTRIBUTING.md` - how a scenario is added to the catalog.
- `scenarios/INDEX.md` - the scenario catalog.
- `scenarios/<id>/scenario.yaml` - one scenario descriptor.
- `scenarios/<id>/...` - the files that scenario reviews.

Running the scenarios is deliberately not part of the bootstrap. This checkout
is the input a review run consumes, never the run itself.
