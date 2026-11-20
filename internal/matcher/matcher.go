package matcher

import (
	"fmt"
	"opforjellyfin/internal/shared"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// MatchFile returns the relative NFO path for a video filename.
func MatchFile(filename string, index *shared.MetadataIndex) (string, error) {
	if index == nil {
		return "", fmt.Errorf("metadata index is missing")
	}
	filename = path.Base(strings.ReplaceAll(filename, `\`, "/"))
	if !shared.IsVideoFile(filename) {
		return "", fmt.Errorf("unsupported video filename: %s", filename)
	}
	id, found := index.ByFilename[strings.TrimSuffix(filename, path.Ext(filename))]
	if !found {
		return "", fmt.Errorf("no metadata for %s", filename)
	}
	entry, found := index.Entries[id]
	if !found || id == "" {
		return "", fmt.Errorf("metadata entry is missing for %s", filename)
	}
	nfoPath := entry.NFOPath
	if path.Ext(nfoPath) != ".nfo" || path.IsAbs(nfoPath) ||
		strings.HasPrefix(nfoPath, "../") || strings.ContainsAny(nfoPath, "\\:\x00") ||
		path.Clean(nfoPath) != nfoPath {
		return "", fmt.Errorf("invalid metadata path: %q", nfoPath)
	}
	return nfoPath, nil
}

// MatchAndPlaceVideo moves or hardlinks a video into its arc folder without renaming it.
// An empty message means the file was already in place.
func MatchAndPlaceVideo(videoPath, targetDir string, index *shared.MetadataIndex, hardlink bool) (string, error) {
	source, err := os.Lstat(videoPath)
	if err != nil {
		return "", err
	}
	if !source.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", videoPath)
	}
	nfo, err := MatchFile(filepath.Base(videoPath), index)
	if err != nil {
		return "", err
	}
	nfoPath := filepath.Join(targetDir, filepath.FromSlash(nfo))
	if !shared.FileExists(nfoPath) {
		return "", fmt.Errorf("metadata file is missing: %s; run 'opfor sync'", nfoPath)
	}
	destination := filepath.Join(filepath.Dir(nfoPath), filepath.Base(videoPath))
	alreadyPlaced := false
	if existing, err := os.Stat(destination); err == nil {
		if os.SameFile(source, existing) {
			alreadyPlaced = true
		} else {
			return "", fmt.Errorf("destination already exists: %s", destination)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// Indexed aliases need matching sidecar names; the video keeps its name.
	metadataStem := strings.TrimSuffix(nfoPath, ".nfo")
	videoStem := strings.TrimSuffix(destination, filepath.Ext(destination))
	if metadataStem != videoStem {
		for _, suffix := range []string{".nfo", "-thumb.png"} {
			if suffix != ".nfo" && !shared.FileExists(metadataStem+suffix) {
				continue
			}
			if err := shared.CopyFile(metadataStem+suffix, videoStem+suffix, 0644); err != nil {
				return "", fmt.Errorf("copy alias metadata: %w", err)
			}
		}
	}
	if alreadyPlaced {
		return "", nil
	}
	action := "Placed"
	if hardlink {
		if err := os.Link(videoPath, destination); err != nil {
			return "", fmt.Errorf("create hardlink (both paths must be on the same filesystem with hardlink support): %w", err)
		}
		action = "Linked"
	} else if err := shared.SafeMoveFile(videoPath, destination); err != nil {
		return "", err
	}
	relative, _ := filepath.Rel(targetDir, destination)
	return fmt.Sprintf("%s: %s", action, relative), nil
}

// Removed is a video RemoveSuperseded deleted, relative to the target directory.
type Removed struct {
	Path     string
	Standard bool // a standard cut replaced by an extended version, not an older release
}

// RemoveSuperseded deletes videos a just-placed video replaces, from the same
// arc folder: older releases of its index entry once it is a latest release,
// and, with standardCut and an extended video, every release of the standard
// cut of the same episode. Alternate versions and other episodes are kept. The
// NFO and thumbnail copies made for removed videos go with them; each entry's
// own metadata files are kept.
func RemoveSuperseded(placedName, targetDir string, index *shared.MetadataIndex, standardCut bool) ([]Removed, error) {
	stem := shared.VideoStem(placedName)
	id, ok := index.ByFilename[stem]
	if !ok {
		return nil, nil
	}
	placed := index.Entries[id]
	latest := index.IsLatestRelease(stem)
	standardCut = standardCut && placed.Variant == "extended"
	if !latest && !standardCut {
		return nil, nil
	}
	dir := filepath.Join(targetDir, filepath.FromSlash(path.Dir(placed.NFOPath)))
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(files, func(f os.DirEntry) bool { return shared.VideoStem(f.Name()) == stem && shared.IsVideoFile(f.Name()) }) {
		return nil, nil // the placed video is not actually in place
	}
	var removed []Removed
	for _, file := range files {
		old := shared.VideoStem(file.Name())
		if !file.Type().IsRegular() || !shared.IsVideoFile(file.Name()) || old == stem {
			continue
		}
		oldID, known := index.ByFilename[old]
		if !known {
			continue
		}
		entry := index.Entries[oldID]
		superseded := latest && oldID == id && !index.IsLatestRelease(old)
		standard := standardCut && entry.EpisodeID == placed.EpisodeID && entry.Kind == placed.Kind &&
			(entry.Variant == "standard" || entry.Variant == "")
		if !superseded && !standard {
			continue
		}
		if err := os.Remove(filepath.Join(dir, file.Name())); err != nil {
			return removed, err
		}
		relative, _ := filepath.Rel(targetDir, filepath.Join(dir, file.Name()))
		removed = append(removed, Removed{Path: relative, Standard: standard})
		if old == strings.TrimSuffix(path.Base(entry.NFOPath), ".nfo") {
			continue
		}
		for _, suffix := range []string{".nfo", "-thumb.png"} {
			if err := os.Remove(filepath.Join(dir, old+suffix)); err != nil && !os.IsNotExist(err) {
				return removed, err
			}
		}
	}
	return removed, nil
}
