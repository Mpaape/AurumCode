#!/usr/bin/env bash
# Bounded, non-sensitive fixture corpus for the AUR-363 Bash adapter lock.
# It exists only to be digested, never parsed.

# greet prints a static greeting.
greet() {
  printf 'hello, %s\n' "$1"
}
