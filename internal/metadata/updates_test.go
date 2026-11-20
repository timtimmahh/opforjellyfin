package metadata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"opforjellyfin/internal/shared"
)

func updatesIndex() *shared.MetadataIndex {
	current := func(name, released string) shared.MetadataRelease {
		return shared.MetadataRelease{Filename: name, Status: "current", Released: released}
	}
	archived := func(name string) shared.MetadataRelease {
		return shared.MetadataRelease{Filename: name, Status: "archived", Released: "2022-01-01"}
	}
	return &shared.MetadataIndex{
		Arcs: map[string]shared.MetadataArc{"arc": {Folder: "Arc [1080p]"}},
		ByFilename: map[string]string{
			"A v2 [1080p][22222222]": "a", "A v1 [720p][11111111]": "a",
			"B [1080p][33333333]":    "b",
			"C [1080p][44444444]":    "c",
			"Old D [480p][55555555]": "d", // no release data: treated as latest
		},
		Entries: map[string]shared.MetadataFile{
			"a": {NFOPath: "Arc [1080p]/A v1 [720p][11111111].nfo", Releases: []shared.MetadataRelease{
				current("A v2 [1080p][22222222]", "2026-11-03 13:28:19"), archived("A v1 [720p][11111111]")}},
			"b": {NFOPath: "Arc [1080p]/B [1080p][33333333].nfo", Releases: []shared.MetadataRelease{
				current("B [1080p][33333333]", "2024-05-01")}},
			"c": {NFOPath: "Arc [1080p]/C [1080p][44444444].nfo", Releases: []shared.MetadataRelease{
				current("C [1080p][44444444]", "2024-05-01")}},
			"d": {NFOPath: "Arc [1080p]/Old D [480p][55555555].nfo"},
		},
	}
}

func TestReleaseUpdate(t *testing.T) {
	index := updatesIndex()
	target := t.TempDir()
	arc := filepath.Join(target, "Arc [1080p]")
	if err := os.MkdirAll(arc, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A v1 [720p][11111111].mkv", "B [1080p][33333333].mkv", "Old D [480p][55555555].mp4", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(arc, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	owned, err := OwnedReleases(target, index)
	if err != nil {
		t.Fatal(err)
	}
	afterAll := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		title string
		date  time.Time
		want  Update
	}{
		{"A v2 [1080p][22222222].mkv", time.Time{}, UpdateRereleased},
		{"A v1 [720p][11111111].mkv", time.Time{}, UpdateNone}, // superseded release
		{"B [1080p][33333333].mkv", time.Time{}, UpdateNone},   // already have latest
		{"C [1080p][44444444].mkv", time.Time{}, UpdateNew},
		{"Old D [480p][55555555].mkv", time.Time{}, UpdateNone},
		{"Unknown [1080p][66666666].mkv", time.Time{}, UpdateNone},
		{"Arc [1080p]", afterAll, UpdateNew},                    // C is new, A re-released
		{"Arc [1080p]", afterAll.AddDate(0, 0, -2), UpdateNone}, // batch predates A v2
		{"Arc [1080p]", time.Time{}, UpdateNone},
	} {
		if got := ReleaseUpdate(shared.TorrentEntry{Title: tc.title, Date: tc.date}, index, owned, nil); got != tc.want {
			t.Errorf("%s (%v): got %v, want %v", tc.title, tc.date, got, tc.want)
		}
	}
}

func TestReleaseUpdateNewWhenNothingOwned(t *testing.T) {
	got := ReleaseUpdate(shared.TorrentEntry{Title: "A v2 [1080p][22222222].mkv"}, updatesIndex(), nil, nil)
	if got != UpdateNew {
		t.Fatalf("got %v", got)
	}
}

func TestReleaseUpdateSkipsStandardCutsCoveredByExtended(t *testing.T) {
	index := &shared.MetadataIndex{
		Arcs: map[string]shared.MetadataArc{"arc": {Folder: "Arc"}},
		ByFilename: map[string]string{
			"X": "x", "X ext": "xe", "X alt": "xa", "Y": "y", "Y ext": "ye",
		},
		Entries: map[string]shared.MetadataFile{
			"x":  {NFOPath: "Arc/X.nfo", EpisodeID: "x", Kind: "episode", Variant: "standard"},
			"xe": {NFOPath: "Arc/X ext.nfo", EpisodeID: "x", Kind: "episode", Variant: "extended"},
			"xa": {NFOPath: "Arc/X alt.nfo", EpisodeID: "x", Kind: "episode", Variant: "alternate-g-8"},
			"y":  {NFOPath: "Arc/Y.nfo", EpisodeID: "y", Kind: "episode", Variant: "standard"},
			"ye": {NFOPath: "Arc/Y ext.nfo", EpisodeID: "y", Kind: "episode", Variant: "extended"},
		},
	}
	owned := map[string][]string{"xe": {"X ext"}, "y": {"Y"}}
	covered := CoveredByExtended(index, owned)
	if len(covered) != 1 || !covered["x"] {
		t.Fatalf("covered = %v", covered)
	}
	for title, want := range map[string]Update{
		"X.mkv":     UpdateNone, // covered by the owned extended version
		"X alt.mkv": UpdateNew,  // alternate versions are not covered
		"Y ext.mkv": UpdateNew,  // the extended version of an owned standard cut
		"Arc":       UpdateNew,  // the batch still has X alt and Y ext
	} {
		if got := ReleaseUpdate(shared.TorrentEntry{Title: title}, index, owned, covered); got != want {
			t.Errorf("%s: got %v, want %v", title, got, want)
		}
	}
	if got := ReleaseUpdate(shared.TorrentEntry{Title: "X.mkv"}, index, owned, nil); got != UpdateNew {
		t.Errorf("without skip: got %v", got)
	}
}
