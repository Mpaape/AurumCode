//! Bounded, non-sensitive fixture corpus for the AUR-363 Rust adapter lock.
//! It exists only to be digested, never parsed.

/// Returns a static greeting.
pub fn greet(name: &str) -> String {
    format!("hello, {name}")
}
