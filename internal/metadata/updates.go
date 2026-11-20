package metadata

import (
	"os"
	"path"
	"path/filepath"

	"opforjellyfin/internal/shared"
)

// Update says why a torrent is worth downloading.
type Update int

const (
	UpdateNone       Update = iota
	UpdateNew               // an episode with no video in the library yet
	UpdateRereleased        // only older releases of the episode are in the library
)

func (u Update) String() string {
	switch u {
	case UpdateNew:
		return "new"
	case UpdateRereleased:
		return "re-release"
	}
	return ""
}

// OwnedReleases maps each entry ID to the filename stems of its videos in the library.
func OwnedReleases(targetDir string, index *shared.MetadataIndex) (map[string][]string, error) {
	owned := make(map[string][]string)
	for _, arc := range index.Arcs {
		files, err := os.ReadDir(filepath.Join(targetDir, arc.Folder))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, file := range files {
			if !file.Type().IsRegular() || !shared.IsVideoFile(file.Name()) {
				continue
			}
			stem := shared.VideoStem(file.Name())
			id, exists := index.ByFilename[stem]
			if exists && path.Dir(index.Entries[id].NFOPath) == arc.Folder {
				owned[id] = append(owned[id], stem)
			}
		}
	}
	return owned, nil
}

// ReleaseUpdate classifies a torrent against the library. Only latest releases
// qualify. A single-episode torrent is new when the episode has no video, and a
// re-release when every video of it is an older release. An arc batch qualifies
// when one of its episodes does and it was uploaded after the arc's newest
// release, since its file list is not known without fetching the torrent.
// Entries in skip are treated as not needed.
func ReleaseUpdate(t shared.TorrentEntry, index *shared.MetadataIndex, owned map[string][]string, skip map[string]bool) Update {
	stem := shared.VideoStem(t.Title)
	if id, ok := index.ByFilename[stem]; ok {
		if skip[id] || !index.IsLatestRelease(stem) {
			return UpdateNone
		}
		return entryUpdate(id, index, owned)
	}
	for _, arc := range index.Arcs {
		if arc.Folder != stem {
			continue
		}
		result := UpdateNone
		for id, entry := range index.Entries {
			if path.Dir(entry.NFOPath) != arc.Folder || skip[id] {
				continue
			}
			if released, ok := entry.LatestReleaseTime(); ok && (t.Date.IsZero() || t.Date.Before(released)) {
				return UpdateNone // the batch predates a release it would need
			}
			// New episodes outrank re-releases when labelling a batch.
			if update := entryUpdate(id, index, owned); update == UpdateNew || result == UpdateNone {
				result = update
			}
		}
		return result
	}
	return UpdateNone
}

func entryUpdate(id string, index *shared.MetadataIndex, owned map[string][]string) Update {
	stems := owned[id]
	if len(stems) == 0 {
		return UpdateNew
	}
	for _, stem := range stems {
		if index.IsLatestRelease(stem) {
			return UpdateNone
		}
	}
	return UpdateRereleased
}

// CoveredByExtended returns the standard cut entry IDs whose episode is owned
// as an extended version, in any release.
func CoveredByExtended(index *shared.MetadataIndex, owned map[string][]string) map[string]bool {
	extended := make(map[string]bool) // episode IDs owned as an extended version
	for id := range owned {
		if entry := index.Entries[id]; entry.Variant == "extended" {
			extended[entry.Kind+"\x00"+entry.EpisodeID] = true
		}
	}
	covered := make(map[string]bool)
	for id, entry := range index.Entries {
		if (entry.Variant == "standard" || entry.Variant == "") && extended[entry.Kind+"\x00"+entry.EpisodeID] {
			covered[id] = true
		}
	}
	return covered
}
