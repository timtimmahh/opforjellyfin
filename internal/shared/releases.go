package shared

import (
	"path"
	"strings"
	"time"
)

const ReleaseCurrent = "current"

// VideoStem returns a filename without directories or a video extension.
func VideoStem(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if IsVideoFile(name) {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}

// HasCurrentRelease reports whether the index knows a current release for the entry.
func (e MetadataFile) HasCurrentRelease() bool {
	for _, release := range e.Releases {
		if release.Status == ReleaseCurrent {
			return true
		}
	}
	return false
}

// IsLatestRelease reports whether a filename stem is a current release of its
// episode. Without release data for the entry (older indexes, untracked
// episodes) nothing newer is known, so every filename counts as latest.
func (ix *MetadataIndex) IsLatestRelease(stem string) bool {
	id, ok := ix.ByFilename[stem]
	if !ok {
		return false
	}
	entry := ix.Entries[id]
	if !entry.HasCurrentRelease() {
		return true
	}
	for _, release := range entry.Releases {
		if release.Filename == stem {
			return release.Status == ReleaseCurrent
		}
	}
	return false
}

// LatestReleaseTime returns when the entry's newest current release came out.
func (e MetadataFile) LatestReleaseTime() (time.Time, bool) {
	var latest time.Time
	for _, release := range e.Releases {
		if release.Status != ReleaseCurrent {
			continue
		}
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, release.Released); err == nil {
				if t.After(latest) {
					latest = t
				}
				break
			}
		}
	}
	return latest, !latest.IsZero()
}
