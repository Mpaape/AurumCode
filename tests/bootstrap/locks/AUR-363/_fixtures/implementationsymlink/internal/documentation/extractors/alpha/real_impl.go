package alpha

// real_impl.go is the symlink target used by the implementationsymlink
// fixture. It is never referenced directly by the lock; only the symlink
// tests/bootstrap/locks/AUR-363/fixtures/implementationsymlink/internal/documentation/extractors/alpha/extractor.go
// is declared as the implementation_path, so the acceptance oracle's
// `-L` guard must reject it before any digest is ever compared.
type AlphaExtractor struct{}
