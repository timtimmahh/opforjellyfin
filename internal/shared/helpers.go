package shared

import (
	"io"
	"os"
	"path/filepath"
)

// copyFile copies from src to dst with permissions using io.Copy. use os.Stat for permissions or 0644
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return os.Chmod(dst, perm)
}

// used by Metadata.go
func RangesOverlap(a1, a2, b1, b2 int) bool {
	return a1 <= b2 && b1 <= a2
}

// file
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
