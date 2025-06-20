// cmd/list.go
package cmd

import (
	"fmt"
	"opforjellyfin/internal"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	rangeFilter  string
	titleFilter  string
	onlySpecials bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available One Pace seasons and specials",
	Run: func(cmd *cobra.Command, args []string) {
		allTorrents, err := internal.FetchOnePaceTorrents()
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}

		// Apply filters after keys are assigned
		var filtered []internal.TorrentEntry
		for _, t := range allTorrents {
			if applyFilters(t) {
				filtered = append(filtered, t)
			}
		}

		// Sort by SeasonKey, then seeders descending
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].DownloadKey == filtered[j].DownloadKey {
				return filtered[i].Seeders > filtered[j].Seeders
			}
			return filtered[i].DownloadKey < filtered[j].DownloadKey
		})

		alternate := false

		fmt.Println("📚 Filtered Download List:\n")
		for _, t := range filtered {

			// bools
			haveMark := "❌"
			metaMark := "❌"

			if t.HaveIt {
				haveMark = "✅"
			}
			if t.MetaDataAvail {
				metaMark = "✅"
			}

			truncatedTitle := truncate(t.SeasonName, 20)

			row := internal.RenderRow(
				"%s - %s: %-30s Have? %s | Meta: %s | %-9s | %-5s | %-3s seeders",
				alternate,
				internal.StyleFactory("DKEY", internal.Style.LBlue),
				internal.StyleFactory(fmt.Sprintf("%4d", t.DownloadKey), internal.Style.Pink),
				internal.StyleFactory(truncatedTitle, internal.Style.LBlue),
				haveMark,
				metaMark,
				t.ChapterRange,
				t.Quality,
				internal.StyleByRange(t.Seeders, 0, 10),
			)

			fmt.Println(row)

			alternate = !alternate
		}
	},
}

func applyFilters(t internal.TorrentEntry) bool {
	// --specials only
	if onlySpecials && !t.IsSpecial {
		return false
	}
	// range filter
	if rangeFilter != "" {
		parts := strings.Split(rangeFilter, "-")
		if len(parts) == 2 {
			min, _ := strconv.Atoi(parts[0])
			max, _ := strconv.Atoi(parts[1])
			if t.DownloadKey < min || t.DownloadKey > max {
				return false
			}
		}
	}
	// title filter
	if titleFilter != "" && !strings.Contains(strings.ToLower(t.SeasonName), strings.ToLower(titleFilter)) {
		return false
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func init() {
	listCmd.Flags().StringVarP(&rangeFilter, "range", "r", "", "Show seasons in range, e.g. 10-20")
	listCmd.Flags().StringVarP(&titleFilter, "title", "t", "", "Filter by title keyword")
	listCmd.Flags().BoolVarP(&onlySpecials, "specials", "s", false, "Show only specials")
	rootCmd.AddCommand(listCmd)
}
