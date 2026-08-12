# Adding a scenario

A scenario is a directory under `scenarios/` with a `scenario.yaml` descriptor
and the files it asks a reviewer to look at.

1. Create `scenarios/<id>/scenario.yaml`. The id is lowercase, starts with a
   letter, and uses `-` as its only separator.
2. Add the reviewed files under the same directory.
3. Register the scenario in `scenarios/INDEX.md`.
4. Reseal the scaffold allowlist so the new paths are copied.

Rules every scenario file must satisfy, because the consumer bootstrap refuses
the whole checkout otherwise:

- the payload is synthetic. A placeholder credential is written as
  `AURUM-FAKE-<NAME>`; a value shaped like a real credential is refused;
- no path may name an AurumCode-private root such as the board, the git
  directory, or an internal source tree;
- no payload may carry an AurumCode-private marker.
