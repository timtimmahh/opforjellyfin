package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func dateCell(t *testing.T, td string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table><tr>" + td + "</tr></table>"))
	if err != nil {
		t.Fatal(err)
	}
	return doc.Find("td")
}

func TestParseUploadDate(t *testing.T) {
	want := time.Date(2026, 11, 3, 13, 28, 0, 0, time.UTC)
	cases := map[string]struct {
		td   string
		want time.Time
	}{
		"timestamp":     {`<td data-timestamp="1793712480">ignored</td>`, want},
		"bad timestamp": {`<td data-timestamp="x"> 2026-11-03 13:28 </td>`, want},
		"text only":     {`<td>2026-11-03 13:28</td>`, want},
		"unparseable":   {`<td>yesterday</td>`, time.Time{}},
		"empty":         {`<td></td>`, time.Time{}},
	}
	for name, c := range cases {
		if got := parseUploadDate(dateCell(t, c.td)); !got.Equal(c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}
