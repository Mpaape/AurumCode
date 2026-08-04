//go:build unix

package cpp

import "syscall"

// openNoFollow makes open(2) fail with ELOOP when the final component of the
// path is a symbolic link, instead of silently writing to the link target.
//
// O_EXCL already refuses any path that exists, a symlink included, so this is a
// second independent barrier rather than the only one.
const openNoFollow = syscall.O_NOFOLLOW
