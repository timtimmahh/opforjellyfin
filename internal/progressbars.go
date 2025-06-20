// internal/progressbars.go
package internal

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type DownloadGui struct {
	Bar *mpb.Bar
}

// unused
func CreateProgressBar(p *mpb.Progress, td *TorrentDownload) *mpb.Bar {
	return p.New(
		td.TotalSize,
		mpb.BarStyle().
			Filler("▓").
			Padding("░").
			Rbound("█"),
		mpb.PrependDecorators(
			decor.Name(truncate(td.Title, 20)),
		),
		mpb.AppendDecorators(
			decor.CountersKibiByte("% .0f / % .0f"),
			decor.Percentage(decor.WCSyncSpace),
			decor.Any(func(decor.Statistics) string {
				return td.Message
			}),
		),
	)
}

func FollowProgress() {

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(loadActiveDownloadsFromFile()) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	downloads := loadActiveDownloadsFromFile()
	if len(downloads) == 0 {
		fmt.Println("📭 No active downloads.")
		return
	}

	lastMessages := make(map[int]string)
	messages := make(map[int]*string)
	bars := make(map[int]*mpb.Bar)

	p := mpb.New(mpb.WithWidth(40))

	for _, td := range downloads {
		messages[td.TorrentID] = &td.Message
		bar := p.New(
			td.TotalSize,
			mpb.BarStyle().Lbound("[").Filler("▓").Tip("█").Padding("░").Rbound("]"),
			mpb.PrependDecorators(
				decor.Name(truncate(td.Title, 15)),
			),
			mpb.AppendDecorators(
				decor.OnComplete(
					decor.Percentage(decor.WCSyncSpace),
					"✔️ Done",
				),
			),
		)
		bars[td.TorrentID] = bar
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	done := make(chan struct{})
	go func() {
		p.Wait()
		close(done)
	}()

	for {
		select {
		case <-ticker.C:
			downloads = loadActiveDownloadsFromFile()
			for _, td := range downloads {
				bar, ok := bars[td.TorrentID]
				if !ok {
					continue
				}

				if td.Done {

					if !bar.Completed() {
						bar.SetTotal(td.TotalSize, true)
					}

				} else {

					bar.SetCurrent(td.Progress)
					bar.SetTotal(td.TotalSize, false)
					if msgPtr, ok := messages[td.TorrentID]; ok && td.Message != lastMessages[td.TorrentID] {
						*msgPtr = td.Message
						lastMessages[td.TorrentID] = td.Message
					}
				}
			}

		case <-signalChan:
			fmt.Println("\n🛑 Cancelled by user.")

			ClearActiveDownloads()
			return

		case <-done:

			fmt.Println("\n✅ All downloads finished.")

			ClearActiveDownloads()
			return
		}
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}
