Fixture: VULNERABLE
Rule: SCR-004 — Vulnerable and Outdated Components (CWE-1104)

The dependency manifest pins a floating range instead of a resolved,
digest-bound version, so the build crosses the network/registry boundary
without a fixed artifact to verify against.

```
require example.com/vendor/parser latest
```

A compromised or yanked release of `parser` is pulled into every future
build without review, and no lock file records what was actually built.
