# Security policy (benchmark)

## SQL Injection
severity: error
Never build a SQL statement by concatenating or formatting untrusted input; use bound parameters.

## Command Injection
severity: error
Never pass untrusted input to a shell or to a process launcher as a single command string.

## Weak Hash
severity: error
Never use MD5 or SHA-1 for passwords or integrity of secrets; use a password hashing function.

## Path Traversal
severity: error
Never join untrusted input into a filesystem path without confining it to a base directory.
