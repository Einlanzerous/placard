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
	results := checker.CheckAll(ctx, entries, cfg.CanonicalBase(), nil)

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	failing := 0
	for _, c := range results {
		if c.OK {
			fmt.Fprintf(w, "ok\t%s\n", c.URL)
			continue
		}
		failing++
		reason := "unknown"
		if c.Error != nil {
			reason = *c.Error
		}
		fmt.Fprintf(w, "FAIL\t%s\t%s\n", c.URL, reason)
	}
	w.Flush()

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
		return fmt.Errorf("%d of %d canonical URLs failing", failing, len(results))
	}
	fmt.Printf("all %d canonical URLs healthy\n", len(results))
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
