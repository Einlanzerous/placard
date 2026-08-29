package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	placard "github.com/Einlanzerous/placard"
	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/checker"
	"github.com/Einlanzerous/placard/internal/config"
	"github.com/Einlanzerous/placard/internal/store"
)

// runCheck is the one-shot verification pass: print every canonical URL's
// health, record the results when a database is configured, and exit non-zero
// if anything is failing — cron- and CI-friendly.
func runCheck() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	entries, err := catalog.Build(placard.Assets)
	if err != nil {
		return err
	}
	ctx := context.Background()
	results := append(checker.CheckAll(ctx, entries, cfg.CanonicalBase(), nil),
		checker.CheckEdge(ctx, entries, cfg.PublicBaseURL, nil)...)

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	failing := 0
	for _, c := range results {
		if c.OK {
			fmt.Fprintf(w, "ok\t%s\t%s\n", c.Kind, c.URL)
			continue
		}
		failing++
		reason := "unknown"
		if c.Error != nil {
			reason = *c.Error
		}
		fmt.Fprintf(w, "FAIL\t%s\t%s\t%s\n", c.Kind, c.URL, reason)
	}

	// The shape report (PRSR-44). A mark a square launcher tile will crop is
	// reported and never failed: the file serves perfectly, every consumer that
	// *fits* rather than fills shows it correctly, and a letterboxed icon still
	// beats two grey initials. Counting it in `failing` would take this command
	// non-zero — and it is run on a cron and in CI — over an asset that works.
	//
	// It needs no network and no database, so it prints even when every URL
	// check above has failed. That is the point of measuring the committed
	// bytes rather than a fetch: the answer is available on a PR, before
	// jsDelivr has heard of the commit.
	// One line per file a human would edit, not per file affected. A
	// raster_from_svg service whose glyph is 3:1 produces four bad PNGs — two
	// marks and their two -dev siblings — from one SVG, and reporting four
	// findings would overstate the estate's problem and point twice at
	// generated files that must never be hand-edited.
	cropped := 0
	for _, e := range entries {
		for _, f := range e.ShapeFindings() {
			cropped++
			detail := f.Shape.Note()
			if n := len(f.Files); n > 1 {
				detail += fmt.Sprintf(" (%d files derive from this one)", n)
			}
			fmt.Fprintf(w, "note\tshape\t%s\t%s\n", f.Source, detail)
		}
	}
	w.Flush()

	if cropped > 0 {
		fmt.Printf("%d mark source(s) will be cropped by a square launcher tile — reported, not failed\n", cropped)
	}

	if cfg.DatabaseURL != "" {
		st, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer st.Close()
		if err := st.Migrate(ctx); err != nil {
			return err
		}
		for _, c := range results {
			if err := st.RecordCheck(ctx, c); err != nil {
				return err
			}
		}
	}

	if failing > 0 {
		return fmt.Errorf("%d of %d checks failing", failing, len(results))
	}
	fmt.Printf("all %d checks healthy\n", len(results))
	return nil
}

func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("PLACARD_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.Migrate(ctx)
}
