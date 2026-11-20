package torrent

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"opforjellyfin/internal/logger"
	"opforjellyfin/internal/matcher"
	"opforjellyfin/internal/metadata"
	"opforjellyfin/internal/shared"
)

var resolutionTag = regexp.MustCompile(`(?i)\[(\d{3,4})p\]`)

// fileRank orders candidate files for an episode: a current release beats an
// older one, then 1080p or higher beats 720p.
func fileRank(index *shared.MetadataIndex, stem string, resolution int) int {
	rank := 0
	if index.IsLatestRelease(stem) {
		rank += 2
	}
	if resolution >= 1080 {
		rank++
	}
	return rank
}

// bestRanks is the highest rank any known release of each episode can reach, so
// the search can stop once every wanted episode has found its best file.
func bestRanks(index *shared.MetadataIndex) map[string]int {
	best := make(map[string]int)
	for _, entry := range index.Entries {
		key := episodeKey(entry)
		if len(entry.Releases) == 0 {
			best[key] = max(best[key], 3)
			continue
		}
		for _, release := range entry.Releases {
			resolution := videoResolution(release.Filename)
			if resolution != 720 && resolution < 1080 {
				continue
			}
			best[key] = max(best[key], fileRank(index, release.Filename, resolution))
		}
	}
	return best
}

// SelectOptions widens what 'download all' selects beyond missing standard episodes.
type SelectOptions struct {
	Extended       bool // extended versions, alongside the standard cut
	PreferExtended bool // extended versions instead of the standard cut, where one exists
	Specials       bool // specials
	Rereleases     bool // latest releases of episodes owned only as an older release; with PreferExtended, also extended versions of owned standard cuts
	KeepOld        bool // with Rereleases, keep the older files instead of removing them
}

// SelectAllDownloads picks one file per missing episode: the latest release when
// a torrent has it, preferring 1080p or higher over 720p, then the most seeders.
// With Rereleases, episodes owned only as older releases are included, and for
// those only a latest release is accepted. With PreferExtended, an episode that
// has an extended version gets that instead of its standard cut, falling back to
// the standard cut when no torrent has the extended version; owning either
// version of an extended episode counts as owning the episode.
func SelectAllDownloads(ctx context.Context, entries []shared.TorrentEntry, cfg *shared.Config, index *shared.MetadataIndex, opts SelectOptions) ([]shared.TorrentEntry, error) {
	if index == nil || len(index.Entries) == 0 {
		return nil, fmt.Errorf("metadata index is missing; run 'opfor sync'")
	}
	wanted := make(map[string]bool)
	hasExtended := make(map[string]bool) // standard keys with a wanted extended version
	for _, entry := range index.Entries {
		if entry.EpisodeID == "" || (entry.Kind != "episode" && entry.Kind != "special") {
			return nil, fmt.Errorf("metadata index lacks episode information; run 'opfor sync'")
		}
		if entry.Kind == "special" && !opts.Specials || entry.Variant == "extended" && !opts.Extended && !opts.PreferExtended {
			continue
		}
		wanted[episodeKey(entry)] = true
		if entry.Variant == "extended" && opts.PreferExtended {
			hasExtended[entry.EpisodeID] = true
		}
	}
	owned, err := metadata.OwnedReleases(cfg.TargetDir, index)
	if err != nil {
		return nil, fmt.Errorf("read existing videos: %w", err)
	}
	// An owned stem counts as latest when the entry has no newer release known.
	ownsLatest := make(map[string]bool)
	for id, stems := range owned {
		key := episodeKey(index.Entries[id])
		ownsLatest[key] = ownsLatest[key] || slices.ContainsFunc(stems, index.IsLatestRelease)
	}
	best := bestRanks(index)
	found := make(map[string]int) // best rank found so far per episode
	rerelease := make(map[string]bool)
	for key, latest := range ownsLatest {
		if opts.Rereleases && !latest && wanted[key] {
			rerelease[key] = true
		} else {
			delete(wanted, key)
		}
	}
	for standard := range hasExtended {
		extended := standard + ":extended"
		_, ownsExtended := ownsLatest[extended]
		_, ownsStandard := ownsLatest[standard]
		switch {
		case ownsExtended:
			delete(wanted, standard) // any extended release covers the standard cut
		case ownsStandard && !opts.Rereleases:
			delete(wanted, extended) // replacing an owned standard cut needs --rereleases
		}
	}
	done := func(key string) bool {
		rank, ok := found[key]
		return ok && rank >= best[key]
	}
	complete := func() bool {
		for key := range wanted {
			if !done(key) && !(hasExtended[key] && done(key+":extended")) {
				return false
			}
		}
		return true
	}

	entries = append([]shared.TorrentEntry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Seeders != entries[j].Seeders {
			return entries[i].Seeders > entries[j].Seeders
		}
		return entries[i].TorrentID < entries[j].TorrentID
	})
	seen := make(map[int]bool)
	var candidates []shared.TorrentEntry
	var fetchError error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if complete() {
			break
		}
		if entry.Seeders <= 0 || entry.TorrentID <= 0 || seen[entry.TorrentID] {
			continue
		}
		seen[entry.TorrentID] = true
		meta, err := fetchTorrentMeta(ctx, cfg.Source.BaseURL, entry.TorrentID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			fetchError = err
			logger.Log(true, "Skipping torrent %d: %v", entry.TorrentID, err)
			continue
		}
		info, err := meta.UnmarshalInfo()
		if err != nil {
			return nil, fmt.Errorf("torrent %d: %w", entry.TorrentID, err)
		}
		entry.SelectedFiles = nil
		for _, file := range info.UpvertedFiles() {
			filename := strings.Join(append([]string{info.BestName()}, file.BestPath()...), "/")
			resolution := videoResolution(filename)
			if !shared.IsVideoFile(filename) || (resolution != 720 && resolution < 1080) {
				continue
			}
			stem := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
			id, exists := index.ByFilename[stem]
			if !exists {
				continue
			}
			if _, err := matcher.MatchFile(filename, index); err != nil {
				return nil, err
			}
			key := episodeKey(index.Entries[id])
			if !wanted[key] || rerelease[key] && !index.IsLatestRelease(stem) {
				continue
			}
			entry.SelectedFiles = append(entry.SelectedFiles, filename)
			rank := fileRank(index, stem, resolution)
			if previous, seenBefore := found[key]; !seenBefore || rank > previous {
				found[key] = rank
			}
		}
		if len(entry.SelectedFiles) > 0 {
			candidates = append(candidates, entry)
		}
	}

	var selected []shared.TorrentEntry
	for _, entry := range candidates {
		files := entry.SelectedFiles
		entry.SelectedFiles = nil
		for _, filename := range files {
			stem := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
			key := episodeKey(index.Entries[index.ByFilename[stem]])
			if !wanted[key] || fileRank(index, stem, videoResolution(filename)) < found[key] {
				continue
			}
			if _, extendedFound := found[key+":extended"]; hasExtended[key] && extendedFound {
				continue // the extended version replaces this standard cut
			}
			entry.SelectedFiles = append(entry.SelectedFiles, filename)
			delete(wanted, key)
		}
		if len(entry.SelectedFiles) > 0 {
			entry.RemoveSuperseded = opts.Rereleases && !opts.KeepOld
			entry.RemoveStandardCut = opts.PreferExtended && opts.Rereleases && !opts.KeepOld
			selected = append(selected, entry)
		}
	}
	if len(selected) == 0 && fetchError != nil {
		return nil, fmt.Errorf("could not select downloads: %w", fetchError)
	}
	return selected, nil
}

func videoResolution(filename string) int {
	match := resolutionTag.FindStringSubmatch(path.Base(filename))
	if len(match) != 2 {
		return 0
	}
	height, _ := strconv.Atoi(match[1])
	return height
}

func episodeKey(entry shared.MetadataFile) string {
	if entry.Variant == "extended" {
		return entry.EpisodeID + ":extended"
	}
	return entry.EpisodeID
}
