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

// RemoveSuperseded deletes older releases of the episode a just-placed video
// belongs to, once that video is a latest release. Only videos in the same arc
// folder that map to the same index entry are removed, with the NFO and
// thumbnail copies made for them; the entry's own metadata files are kept.
// It returns the removed videos relative to targetDir.
func RemoveSuperseded(placedName, targetDir string, index *shared.MetadataIndex) ([]string, error) {
	stem := shared.VideoStem(placedName)
	id, ok := index.ByFilename[stem]
	if !ok || !index.IsLatestRelease(stem) {
		return nil, nil
	}
	nfoPath := index.Entries[id].NFOPath
	dir := filepath.Join(targetDir, filepath.FromSlash(path.Dir(nfoPath)))
	canonical := strings.TrimSuffix(path.Base(nfoPath), ".nfo")
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(files, func(f os.DirEntry) bool { return shared.VideoStem(f.Name()) == stem && shared.IsVideoFile(f.Name()) }) {
		return nil, nil // the latest release is not actually in place
	}
	var removed []string
	for _, file := range files {
		old := shared.VideoStem(file.Name())
		if !file.Type().IsRegular() || !shared.IsVideoFile(file.Name()) || old == stem ||
			index.ByFilename[old] != id || index.IsLatestRelease(old) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, file.Name())); err != nil {
			return removed, err
		}
		relative, _ := filepath.Rel(targetDir, filepath.Join(dir, file.Name()))
		removed = append(removed, relative)
		if old == canonical {
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
