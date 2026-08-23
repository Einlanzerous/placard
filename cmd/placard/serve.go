package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	placard "github.com/Einlanzerous/placard"
	"github.com/Einlanzerous/placard/internal/api"
	"github.com/Einlanzerous/placard/internal/catalog"
	"github.com/Einlanzerous/placard/internal/checker"
	"github.com/Einlanzerous/placard/internal/config"
	"github.com/Einlanzerous/placard/internal/store"
)

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	entries, err := catalog.Build(placard.Assets)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The store is optional by design (CLAUDE.md): the asset store must not
	// depend on infrastructure state. A missing database costs the checks and
	// staged uploads, never the marks.
	var st *store.Store
	if cfg.DatabaseURL == "" {
		log.Print("serve: no database configured — canonical-URL checks and staged uploads are off")
	} else {
		st, err = store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer st.Close()
		if err := st.Migrate(ctx); err != nil {
			return err
		}
	}

	if st != nil && cfg.CheckInterval > 0 {
		go checkLoop(ctx, cfg, st, entries)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: api.New(cfg, st, entries, placard.Assets)}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("placard serving on %s (%d services, uploads %s)", cfg.Addr, len(entries),
		map[bool]string{true: "enabled", false: "disabled"}[cfg.UploadToken != "" && st != nil])

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}
}

// checkLoop verifies shortly after boot (giving jsDelivr a beat to pick up a
// fresh push), then on the configured interval.
func checkLoop(ctx context.Context, cfg config.Config, st *store.Store, entries []catalog.Entry) {
	t := time.NewTimer(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		recordChecks(ctx, cfg, st, entries)
		t.Reset(cfg.CheckInterval)
	}
}

func recordChecks(ctx context.Context, cfg config.Config, st *store.Store, entries []catalog.Entry) {
	results := checker.CheckAll(ctx, entries, cfg.CanonicalBase(), nil)
	edge := checker.CheckEdge(ctx, entries, cfg.PublicBaseURL, nil)
	ok, edgeOK := 0, 0
	for _, c := range append(results, edge...) {
		if c.OK {
			if c.Kind == store.KindEdge {
				edgeOK++
			} else {
				ok++
			}
		} else if c.Error != nil {
			log.Printf("check: [%s] %s FAILING: %s (%s)", c.Kind, c.File, *c.Error, c.URL)
		}
		if err := st.RecordCheck(ctx, c); err != nil {
			log.Printf("check: record %s: %v", c.File, err)
			return
		}
	}
	log.Printf("check: %d/%d canonical URLs healthy", ok, len(results))
	if len(edge) > 0 {
		state := "SESSIONLESS FETCHES FAILING — is an Access gate on the hostname? (docs/deploy.md §4)"
		if edgeOK == len(edge) {
			state = "serving openly"
		}
		log.Printf("check: public edge %s (%d/%d)", state, edgeOK, len(edge))
	}
}
