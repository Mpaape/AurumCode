Fixture: FIXED
Rule: SCR-004 — Vulnerable and Outdated Components (CWE-1104)

The manifest pins an exact, maintained version and the lock file binds it
to a content digest, so any change to the resolved artifact is visible and
must be reviewed.

```
require example.com/vendor/parser v2.4.1

// go.sum / lock file:
// example.com/vendor/parser v2.4.1 h1:9f7c...==(sha-256 digest)
```

The build fails closed if the fetched artifact's digest does not match the
one recorded in the lock file.
