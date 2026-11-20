// cmd/list.go
package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"opforjellyfin/internal/flags"
	"opforjellyfin/internal/logger"
	"opforjellyfin/internal/metadata"
	"opforjellyfin/internal/scraper"
	"opforjellyfin/internal/shared"
	"opforjellyfin/internal/ui"

	"github.com/spf13/cobra"
)

var (
	rangeFilter          string
	titleFilter          string
	minimumQualityFilter = flags.StringChoice([]string{"480p", "720p", "1080p"})
	qualityFilter        = flags.StringChoice([]string{"480p", "720p", "1080p"})

	onlySpecials bool
	verboseList  bool
	sortNewest   bool
	onlyUpdates  bool

	alternate bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available One Pace seasons and specials",
	Run: func(cmd *cobra.Command, args []string) {
		spinner := ui.NewMultirowSpinner(ui.Animations["Searcher"], 4)

		cfg, _ := shared.LoadConfig()

		if cfg.Source.BaseURL == "" {
			spinner.Stop()
			logger.Log(true, "⚠️ No valid scraper configuration found. Please run 'sync' or 'setDir'")
			return
		}

		allTorrents, err := scraper.FetchTorrents(cfg)
		if err != nil {
			spinner.Stop()
			logger.Log(true, "❌ Error scraping torrents. Site inaccessible? %v", err)
			return
		}

		// Apply filters after keys are assigned
		var filtered []shared.TorrentEntry
		for _, t := range allTorrents {
			if applyFilters(t) {
				filtered = append(filtered, t)
			}
		}

		updates := map[int]metadata.Update{}
		if onlyUpdates {
			if filtered, updates, err = filterUpdates(cfg, filtered); err != nil {
				spinner.Stop()
				logger.Log(true, "❌ %v", err)
				return
			}
		}

		sortTorrents(filtered, sortNewest)

		spinner.Stop()

		fmt.Println("📚 Filtered Download List:")
		for _, t := range filtered {
			if verboseList {
				renderVerboseRow(t, updates[t.TorrentID])
			} else {
				renderRow(t, updates[t.TorrentID])
			}
		}
	},
}

// filterUpdates keeps latest releases that are missing from the library or
// replace an older release in it, returning why each was kept by torrent ID.
func filterUpdates(cfg *shared.Config, entries []shared.TorrentEntry) ([]shared.TorrentEntry, map[int]metadata.Update, error) {
	index, err := metadata.LoadMetadataCache()
	if err != nil {
		return nil, nil, err
	}
	owned, err := metadata.OwnedReleases(cfg.TargetDir, index)
	if err != nil {
		return nil, nil, fmt.Errorf("read library: %w", err)
	}
	kept := make([]shared.TorrentEntry, 0, len(entries))
	updates := make(map[int]metadata.Update)
	for _, t := range entries {
		if update := metadata.ReleaseUpdate(t, index, owned); update != metadata.UpdateNone {
			kept = append(kept, t)
			updates[t.TorrentID] = update
		}
	}
	return kept, updates, nil
}

func updateTag(update metadata.Update) string {
	switch update {
	case metadata.UpdateNew:
		return " | " + ui.StyleFactory("🆕 new", ui.Style.Green)
	case metadata.UpdateRereleased:
		return " | " + ui.StyleFactory("♻️  re-release", ui.Style.Pink)
	}
	return ""
}

// sortTorrents orders by upload date (newest first, unknown dates last) when
// newest is set, otherwise by download key. Seeders descending breaks ties.
func sortTorrents(entries []shared.TorrentEntry, newest bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if newest && !a.Date.Equal(b.Date) {
			return a.Date.After(b.Date)
		}
		if !newest && a.DownloadKey != b.DownloadKey {
			return a.DownloadKey < b.DownloadKey
		}
		return a.Seeders > b.Seeders
	})
}

func formatUploadDate(date time.Time) string {
	if date.IsZero() {
		return "N/A"
	}
	return date.Format("2006-01-02 15:04:05")
}

func applyFilters(t shared.TorrentEntry) bool {
	// --specials only
	if onlySpecials && !t.IsSpecial {
		return false
	}
	// range filter
	if rangeFilter != "" {

		// defaults
		cMin, cMax, min, max := 0, 0, 0, 0

		// chapterRange of torrent
		cMin, cMax = shared.ParseRange(t.ChapterRange)

		// chosen chapterRange
		min, max = shared.ParseRange(rangeFilter)

		if cMin < 0 || cMax < 0 || !shared.RangesOverlap(cMin, cMax, min, max) {
			return false
		}
	}
	// title filter
	if titleFilter != "" && !strings.Contains(strings.ToLower(t.TorrentName), strings.ToLower(titleFilter)) {
		return false
	}

	if qualityFilter.String() != "" && t.Quality != qualityFilter.Value {
		return false
	}

	if minimumQualityFilter.String() != "" {
		// Not sure about error checking here, both values should be guaranteed parsable if everything else works as intended.
		quality, _ := strconv.Atoi(strings.TrimSuffix(t.Quality, "p"))
		minimumQuality, _ := strconv.Atoi(strings.TrimSuffix(minimumQualityFilter.Value, "p"))
		if quality < minimumQuality {
			return false
		}
	}

	return true
}

// rowrender
func renderRow(t shared.TorrentEntry, update metadata.Update) {
	// bools
	metaMark := "❌"

	haveMark := map[int]string{
		0: "❌",
		1: "🟠",
		2: "✅",
	}[t.HaveIt]

	if t.MetaDataAvail {
		metaMark = "✅"
	}

	// styling and render
	truncatedTitle := ui.AnsiPadRight(t.TorrentName, 30)

	row := ui.RenderRow(
		"%s - %s: %s Have? %s | Meta: %s | %-9s | %s | %s seeders | %s | %s",
		alternate,
		ui.StyleFactory("DKEY", ui.Style.LBlue),
		ui.StyleFactory(fmt.Sprintf("%4d", t.DownloadKey), ui.Style.Pink),
		ui.StyleFactory(truncatedTitle, ui.Style.LBlue),
		haveMark,
		metaMark,
		t.ChapterRange,
		ui.AnsiPadLeft(ui.StyleByRange(t.Quality, 400, 1000), 5),
		ui.AnsiPadLeft(ui.StyleByRange(t.Seeders, 0, 10), 3),
		t.FileSize,
		formatUploadDate(t.Date),
	)

	// set flag
	alternate = !alternate

	fmt.Println(row + updateTag(update))
}

// render verbose row
func renderVerboseRow(t shared.TorrentEntry, update metadata.Update) {
	metaMark := "❌"
	if t.MetaDataAvail {
		metaMark = "✅"
	}

	haveMark := map[int]string{
		0: "❌",
		1: "🟠",
		2: "✅",
	}[t.HaveIt]

	fullTitle := ui.AnsiPadRight(t.Title, 60)

	row := ui.RenderRow(
		"%s - %s: %s H:%s M:%s | %s seeders | %s | %s",
		alternate,
		ui.StyleFactory("DKEY", ui.Style.LBlue),
		ui.StyleFactory(fmt.Sprintf("%4d", t.DownloadKey), ui.Style.Pink),
		ui.StyleFactory(fullTitle, ui.Style.LBlue),
		haveMark,
		metaMark,
		ui.AnsiPadLeft(ui.StyleByRange(t.Seeders, 0, 10), 3),
		t.FileSize,
		formatUploadDate(t.Date),
	)

	alternate = !alternate
	fmt.Println(row + updateTag(update))
}

// init
func init() {
	listCmd.Flags().StringVarP(&rangeFilter, "range", "r", "", "Show seasons in range, e.g. 10-20")
	listCmd.Flags().StringVarP(&titleFilter, "title", "t", "", "Filter by title keyword")
	listCmd.Flags().VarP(qualityFilter, "quality", "q", "Filter by quality, e.g. 1080p")
	listCmd.Flags().Var(minimumQualityFilter, "minquality", "Filter by minimum quality, e.g. 720p will only list 720p and 1080p.")

	listCmd.Flags().BoolVarP(&onlySpecials, "specials", "s", false, "Show only specials")
	listCmd.Flags().BoolVarP(&verboseList, "verbose", "v", false, "Show full titles")
	listCmd.Flags().BoolVarP(&sortNewest, "newest", "n", false, "Sort by torrent upload date, newest first")
	listCmd.Flags().BoolVarP(&onlyUpdates, "updates", "u", false, "Show only latest releases you don't have: new episodes and re-releases of ones you have")
	rootCmd.AddCommand(listCmd)
}
