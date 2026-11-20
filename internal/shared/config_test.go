package shared

import "testing"

func TestWithSourceDefaultsBackfillsOldConfig(t *testing.T) {
	// Shape saved by the old metadata repo: no upload_date or file_size.
	old := ScraperConfig{Name: "nyaa", BaseURL: "https://nyaa.si", Fields: ScraperFields{Title: "custom"}}
	got := withSourceDefaults(old)
	if got.Fields.UploadDate == "" || got.Fields.FileSize == "" {
		t.Fatalf("selectors not backfilled: %+v", got.Fields)
	}
	if got.Fields.Title != "custom" {
		t.Fatalf("existing selector overwritten: %q", got.Fields.Title)
	}
}

func TestWithSourceDefaultsLeavesOtherTrackersAlone(t *testing.T) {
	got := withSourceDefaults(ScraperConfig{Name: "other", BaseURL: "https://example.test"})
	if got.Fields.UploadDate != "" || got.Fields.FileSize != "" {
		t.Fatalf("nyaa selectors applied to another tracker: %+v", got.Fields)
	}
}

func TestWithSourceDefaultsEmptyConfig(t *testing.T) {
	if got := withSourceDefaults(ScraperConfig{}); got.BaseURL != defaultSource.BaseURL {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRange(t *testing.T) {
	cases := map[string][2]int{
		"15-20": {15, 20},
		"15":    {15, 15},
		"":      {-1, -1}, // specials must stay excluded from range filters
		"abc":   {-1, -1},
	}
	for in, want := range cases {
		if a, b := ParseRange(in); a != want[0] || b != want[1] {
			t.Errorf("ParseRange(%q) = %d,%d, want %v", in, a, b, want)
		}
	}
}

func TestMetadataRepo(t *testing.T) {
	for stored, want := range map[string]string{
		"":                         DefaultMetadataRepo,
		"tissla/one-pace-jellyfin": DefaultMetadataRepo, // legacy format
		" me/my-metadata ":         "me/my-metadata",
	} {
		if got := (&Config{GitHubRepo: stored}).MetadataRepo(); got != want {
			t.Errorf("MetadataRepo(%q) = %q, want %q", stored, got, want)
		}
	}
}

func TestValidateMetadataRepo(t *testing.T) {
	for _, ok := range []string{"tissla/opfor-metadata-barrys", "me/my.repo_1"} {
		if err := ValidateMetadataRepo(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "me", "me/", "/repo", "-me/repo", "me/repo/extra", "me/..", "https://github.com/me/repo", "me/repo --upload-pack=x", "me/re po"} {
		if err := ValidateMetadataRepo(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
