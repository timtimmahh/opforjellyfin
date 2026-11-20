package shared

import (
	"errors"
	"fmt"
	"io"
	"opforjellyfin/internal/logger"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var (
	// Mutex for directory creation and file movement operations
	dirMutex sync.Mutex
)

// helper for tempdir
func GetTempDir() (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}

	if cfg.TargetDir == "" {
		return "", errors.New("No target dir set")
	}
	tmpDir := filepath.Join(cfg.TargetDir, ".opfor-tmp")

	if _, err := os.Stat(tmpDir); err == nil {
		return tmpDir, nil
	}

	//create if not exists
	err = os.MkdirAll(tmpDir, 0755)
	if err != nil {
		return "", err
	}

	return tmpDir, nil

}

// CreateTempTorrentDir safely creates a temporary directory for torrent downloads
func CreateTempTorrentDir(torrentID int) (string, error) {
	dirMutex.Lock()
	defer dirMutex.Unlock()

	tmpBase, err := GetTempDir()
	if err != nil {
		return "", err
	}
	tmpDir := filepath.Join(tmpBase, fmt.Sprintf("opfor-tmp-%d", torrentID))

	// Check if it already exists
	if info, err := os.Stat(tmpDir); err == nil && info.IsDir() {
		logger.Log(false, "Temp dir already exists: %s", tmpDir)
		return tmpDir, nil
	}

	// Create with MkdirAll (safe for concurrent calls)
	if err = os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	logger.Log(false, "Created temp dir: %s", tmpDir)
	return tmpDir, nil
}

// SafeMoveFile copies then removes the source, without overwriting a destination.
func SafeMoveFile(src, dst string) error {
	dirMutex.Lock()
	defer dirMutex.Unlock()
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(dst)
		return errors.Join(copyErr, closeErr)
	}
	if err := in.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func IsVideoFile(filename string) bool {
	ext := filepath.Ext(filename)
	return strings.EqualFold(ext, ".mkv") || strings.EqualFold(ext, ".mp4")
}

// copyFileInternal is the internal non-locked version for use within already locked functions
func copyFileInternal(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Chmod(perm)
}

// CopyFile copies from src to dst with permissions using io.Copy. use os.Stat for permissions or 0644
// This is the public version that locks
func CopyFile(src, dst string, perm os.FileMode) error {
	dirMutex.Lock()
	defer dirMutex.Unlock()

	return copyFileInternal(src, dst, perm)
}

// CreateDirectory safely creates a directory with proper locking
func CreateDirectory(path string) error {
	dirMutex.Lock()
	defer dirMutex.Unlock()

	return os.MkdirAll(path, 0755)
}

// FileExists checks if a file exists (no locking needed for read operation)
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// CopyDir copies all files (overwrites)
func CopyDir(src, dst string) error {
	dirMutex.Lock()
	defer dirMutex.Unlock()

	return walkAndCopyInternal(src, dst, false)
}

// SyncDir copies new/changed files from src to dst, then warns about NFOs in
// dst that src no longer has. index is the metadata index from src; it is used
// to recognize the NFO copies opfor makes for placed videos.
func SyncDir(src, dst string, index *MetadataIndex) error {
	dirMutex.Lock()
	defer dirMutex.Unlock()

	if err := walkAndCopyInternal(src, dst, true); err != nil {
		return err
	}

	stale, err := staleMetadataFiles(src, dst, index)
	if err != nil {
		logger.Log(false, "staleMetadataFiles: walk failed: %v", err)
	}
	for _, relPath := range stale {
		logger.Log(true, "⚠️  %s no longer exists in the metadata repo (kept locally - review manually)", relPath)
	}
	return nil
}

// staleMetadataFiles returns the .nfo files under dst that do not exist in the
// upstream src tree (e.g. the metadata repo renamed or removed an episode),
// relative to dst. It never deletes anything, since dst also holds
// user-downloaded video files that this must never touch.
//
// NFO copies made for placed videos are not stale: when a video's filename is
// an alias of an episode (a newer release, say), placement copies the episode's
// NFO to the video's name, and the repo only has the episode's own NFO. A copy
// is recognized by a video with the same name beside it whose filename the
// index maps to an episode in that folder.
func staleMetadataFiles(src, dst string, index *MetadataIndex) ([]string, error) {
	var stale []string
	err := filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// .opfor-tmp (the in-progress clone, including src itself) lives
			// inside dst - walking into it would compare every freshly
			// cloned NFO against itself under a doubly-nested path and
			// falsely report all of them as stale. Skip dotdirs entirely.
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(info.Name()), ".nfo") {
			return nil
		}

		relPath, relErr := filepath.Rel(dst, path)
		if relErr != nil {
			return nil
		}

		if !FileExists(filepath.Join(src, relPath)) && !isPlacedNFOCopy(dst, relPath, index) {
			stale = append(stale, relPath)
		}

		return nil
	})
	return stale, err
}

// isPlacedNFOCopy reports whether the NFO at relPath under dst belongs to a
// video beside it that the index maps to an episode in the same folder.
func isPlacedNFOCopy(dst, relPath string, index *MetadataIndex) bool {
	if index == nil {
		return false
	}
	stem := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	id, ok := index.ByFilename[stem]
	if !ok {
		return false
	}
	if path.Dir(index.Entries[id].NFOPath) != filepath.ToSlash(filepath.Dir(relPath)) {
		return false
	}
	files, err := os.ReadDir(filepath.Join(dst, filepath.Dir(relPath)))
	if err != nil {
		return false
	}
	for _, file := range files {
		if file.Type().IsRegular() && IsVideoFile(file.Name()) && VideoStem(file.Name()) == stem {
			return true
		}
	}
	return false
}

// walkAndCopyInternal is the internal non-locked version
func walkAndCopyInternal(src, dst string, onlyIfChanged bool) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		if onlyIfChanged && FileExists(destPath) {
			old, err1 := os.ReadFile(destPath)
			new, err2 := os.ReadFile(path)
			if err1 == nil && err2 == nil && string(old) == string(new) {
				return nil
			}
		}

		return copyFileInternal(path, destPath, info.Mode())
	})
}
