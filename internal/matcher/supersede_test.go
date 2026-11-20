package matcher

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"opforjellyfin/internal/shared"
)

func supersedeFixture(t *testing.T) (string, string, *shared.MetadataIndex) {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "library")
	arc := filepath.Join(target, "Arc")
	if err := os.MkdirAll(arc, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"A old.nfo", "A old-thumb.png", "A old.mkv", // canonical metadata and oldest video
		"A mid.mkv", "A mid.nfo", "A mid-thumb.png", // older alias with copied sidecars
		"A recut.nfo", "A recut.mkv", // different entry: must stay
		"B.nfo", "B old.mkv", // other episode's older release: must stay
	} {
		if err := os.WriteFile(filepath.Join(arc, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	rel := func(name, status string) shared.MetadataRelease {
		return shared.MetadataRelease{Filename: name, Status: status}
	}
	index := &shared.MetadataIndex{
		ByFilename: map[string]string{"A new": "a", "A mid": "a", "A old": "a", "A recut": "r", "B": "b", "B old": "b"},
		Entries: map[string]shared.MetadataFile{
			"a": {NFOPath: "Arc/A old.nfo", Releases: []shared.MetadataRelease{rel("A new", "current"), rel("A mid", "archived"), rel("A old", "unknown")}},
			"r": {NFOPath: "Arc/A recut.nfo", Releases: []shared.MetadataRelease{rel("A recut", "current")}},
			"b": {NFOPath: "Arc/B.nfo", Releases: []shared.MetadataRelease{rel("B", "current"), rel("B old", "archived")}},
		},
	}
	download := filepath.Join(root, "download")
	if err := os.Mkdir(download, 0755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(download, "A new.mkv")
	if err := os.WriteFile(video, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	return video, target, index
}

func arcFiles(t *testing.T, target string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(target, "Arc"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestProcessTorrentFilesRemovesSupersededReleases(t *testing.T) {
	video, target, index := supersedeFixture(t)
	td := &shared.TorrentDownload{TorrentID: 9001, VideoPaths: []string{video}, RemoveSuperseded: true}
	if err := ProcessTorrentFiles(target, td, index); err != nil {
		t.Fatal(err)
	}
	want := []string{"A new.mkv", "A new.nfo", "A new-thumb.png", "A old-thumb.png", "A old.nfo", "A recut.mkv", "A recut.nfo", "B old.mkv", "B.nfo"}
	sort.Strings(want)
	if got := arcFiles(t, target); !reflect.DeepEqual(got, want) {
		t.Fatalf("library = %v\nwant      %v", got, want)
	}
	removed := 0
	for _, line := range td.PlacementFull {
		if line == "Removed older release: Arc/A old.mkv" || line == "Removed older release: Arc/A mid.mkv" {
			removed++
		}
	}
	if removed != 2 {
		t.Fatalf("placement messages %v", td.PlacementFull)
	}
}

func TestProcessTorrentFilesKeepsOldReleasesWhenDisabled(t *testing.T) {
	video, target, index := supersedeFixture(t)
	before := len(arcFiles(t, target))
	td := &shared.TorrentDownload{TorrentID: 9002, VideoPaths: []string{video}}
	if err := ProcessTorrentFiles(target, td, index); err != nil {
		t.Fatal(err)
	}
	// Only the new video and its NFO and thumbnail copies are added.
	if got := len(arcFiles(t, target)); got != before+3 {
		t.Fatalf("library = %v", arcFiles(t, target))
	}
}

func TestRemoveSupersededIgnoresOlderReleases(t *testing.T) {
	_, target, index := supersedeFixture(t)
	// Placing an archived release never removes anything.
	removed, err := RemoveSuperseded("A mid.mkv", target, index, true)
	if err != nil || removed != nil {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	// A latest release that is not actually in the folder removes nothing either.
	removed, err = RemoveSuperseded("A new.mkv", target, index, true)
	if err != nil || removed != nil {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
}

func TestProcessTorrentFilesReplacesStandardCut(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "library")
	arc := filepath.Join(target, "Arc")
	if err := os.MkdirAll(arc, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"X.nfo", "X.mkv", // standard cut with its canonical metadata
		"X old.mkv", "X old.nfo", "X old-thumb.png", // older standard release with copied sidecars
		"X ext.nfo",              // extended metadata
		"X alt.nfo", "X alt.mkv", // alternate version: must stay
		"Y.nfo", "Y.mkv", // other episode: must stay
	} {
		if err := os.WriteFile(filepath.Join(arc, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	index := &shared.MetadataIndex{
		ByFilename: map[string]string{"X": "x", "X old": "x", "X ext": "xe", "X alt": "xa", "Y": "y"},
		Entries: map[string]shared.MetadataFile{
			"x":  {NFOPath: "Arc/X.nfo", EpisodeID: "x", Kind: "episode", Variant: "standard"},
			"xe": {NFOPath: "Arc/X ext.nfo", EpisodeID: "x", Kind: "episode", Variant: "extended"},
			"xa": {NFOPath: "Arc/X alt.nfo", EpisodeID: "x", Kind: "episode", Variant: "alternate-g-8"},
			"y":  {NFOPath: "Arc/Y.nfo", EpisodeID: "y", Kind: "episode", Variant: "standard"},
		},
	}
	video := filepath.Join(root, "X ext.mkv")
	if err := os.WriteFile(video, []byte("ext"), 0644); err != nil {
		t.Fatal(err)
	}
	td := &shared.TorrentDownload{TorrentID: 9003, VideoPaths: []string{video}, RemoveStandardCut: true}
	if err := ProcessTorrentFiles(target, td, index); err != nil {
		t.Fatal(err)
	}
	want := []string{"X alt.mkv", "X alt.nfo", "X ext.mkv", "X ext.nfo", "X.nfo", "Y.mkv", "Y.nfo"}
	if got := arcFiles(t, target); !reflect.DeepEqual(got, want) {
		t.Fatalf("library = %v\nwant      %v", got, want)
	}
	removed := 0
	for _, line := range td.PlacementFull {
		if line == "Removed standard cut: Arc/X.mkv" || line == "Removed standard cut: Arc/X old.mkv" {
			removed++
		}
	}
	if removed != 2 {
		t.Fatalf("placement messages %v", td.PlacementFull)
	}
	// Placing a standard cut never removes an extended version.
	if err := os.WriteFile(filepath.Join(arc, "X.mkv"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if removed, err := RemoveSuperseded("X.mkv", target, index, true); err != nil || removed != nil {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
}
