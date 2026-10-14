package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A large text file in the verified checkout of a --pr review is reviewed:
// the content check reads only the prefix it inspects and never declares a
// file unreviewed for its size. A binary file of the same size still is.
func TestUninspectedPRNotices_LargeTextIsReviewed(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("a line of a large text file\n", (2<<20)/28+1)
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := &types.Diff{Files: []types.DiffFile{{Path: "big.txt"}, {Path: "big.bin"}}}

	notices := uninspectedPRNotices(diff, dir)
	if len(notices) != 1 || notices[0].Path != "big.bin" || notices[0].Reason != analyzer.NoticeReasonBinary {
		t.Fatalf("only the binary file may be unreviewed, got %+v", notices)
	}
	data, err := readRegularFile(filepath.Join(dir, "big.txt"))
	if err != nil || len(data) != analyzer.ClassifyInspectBytes {
		t.Fatalf("the content check must read only its %d-byte prefix, read %d (err %v)", analyzer.ClassifyInspectBytes, len(data), err)
	}
}
