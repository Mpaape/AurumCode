//go:build !unix

package cpp

// openNoFollow is zero on platforms that have no O_NOFOLLOW. Creation still uses
// O_CREATE|O_EXCL, which refuses to open a path that already exists, and that is
// what stops the write from being redirected onto a file this process does not
// own.
const openNoFollow = 0
