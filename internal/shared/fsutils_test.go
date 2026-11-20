package shared

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestStaleMetadataFilesSkipsPlacedNFOCopies(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	dst := filepath.Join(root, "library")
	write := func(base, name string) {
		t.Helper()
		p := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(src, "Arc/A.nfo")
	write(src, "Other/B.nfo")
	for _, name := range []string{
		"Arc/A.nfo", "Other/B.nfo", // in the repo
		"Arc/A new.nfo", "Arc/A new.mkv", // copy for a placed newer release: not stale
		"Arc/A old.nfo",                          // copy whose video is gone: stale
		"Other/A moved.nfo", "Other/A moved.mkv", // alias, but in another episode's folder: stale
		"Arc/Unknown.nfo", "Arc/Unknown.mkv", // not in the index: stale
		"Arc/Removed episode.nfo", // removed from the repo: stale
		".opfor-tmp/Arc/X.nfo",    // the in-progress clone is skipped
	} {
		write(dst, name)
	}
	index := &MetadataIndex{
		ByFilename: map[string]string{"A": "a", "A new": "a", "A old": "a", "A moved": "a", "B": "b"},
		Entries: map[string]MetadataFile{
			"a": {NFOPath: "Arc/A.nfo"},
			"b": {NFOPath: "Other/B.nfo"},
		},
	}
	stale, err := staleMetadataFiles(src, dst, index)
	if err != nil {
		t.Fatal(err)
	}
	for i := range stale {
		stale[i] = filepath.ToSlash(stale[i])
	}
	sort.Strings(stale)
	want := []string{"Arc/A old.nfo", "Arc/Removed episode.nfo", "Arc/Unknown.nfo", "Other/A moved.nfo"}
	if !reflect.DeepEqual(stale, want) {
		t.Fatalf("stale = %v\nwant    %v", stale, want)
	}

	// Without an index every NFO missing from the repo is reported, as before.
	all, err := staleMetadataFiles(src, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(want)+1 {
		t.Fatalf("without index: %v", all)
	}
}
