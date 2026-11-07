package cmd

import (
	"testing"
	"time"

	"opforjellyfin/internal/shared"
)

func keys(entries []shared.TorrentEntry) []int {
	out := make([]int, len(entries))
	for i, e := range entries {
		out[i] = e.DownloadKey
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortFixture() []shared.TorrentEntry {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []shared.TorrentEntry{
		{DownloadKey: 1, Date: older, Seeders: 50},
		{DownloadKey: 2, Seeders: 99}, // unknown date
		{DownloadKey: 3, Date: newer, Seeders: 1},
		{DownloadKey: 4, Date: newer, Seeders: 9},
	}
}

func TestSortTorrentsNewestFirst(t *testing.T) {
	entries := sortFixture()
	sortTorrents(entries, true)
	if got, want := keys(entries), []int{4, 3, 1, 2}; !equalInts(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortTorrentsDefaultsToDownloadKey(t *testing.T) {
	entries := sortFixture()
	entries[0], entries[3] = entries[3], entries[0]
	sortTorrents(entries, false)
	if got, want := keys(entries), []int{1, 2, 3, 4}; !equalInts(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestFormatUploadDateUnknown(t *testing.T) {
	if got := formatUploadDate(time.Time{}); got != "N/A" {
		t.Fatalf("got %q", got)
	}
}
