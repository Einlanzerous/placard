package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// DB-backed tests run only where PLACARD_TEST_DATABASE_URL points at a
// disposable database (CI's service container, or placard_test on the box).
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("PLACARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PLACARD_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"mark_check", "staged_upload"} {
		if _, err := s.pool.Exec(ctx, "TRUNCATE "+table); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := testStore(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestChecksLatestWins(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	status, ct := 404, "text/html"
	first := MarkCheck{
		Service: "argosy", File: "argosy/argosy-mark-light.png",
		URL: "https://example.test/a.png", OK: false,
		HTTPStatus: &status, ContentType: &ct,
	}
	if err := s.RecordCheck(ctx, first); err != nil {
		t.Fatal(err)
	}
	// Same file checked again, now healthy. Distinct timestamps matter for
	// DISTINCT ON ordering, so give the clock a tick.
	time.Sleep(10 * time.Millisecond)
	ok200 := 200
	second := first
	second.OK = true
	second.HTTPStatus = &ok200
	if err := s.RecordCheck(ctx, second); err != nil {
		t.Fatal(err)
	}

	latest, err := s.LatestChecks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, present := latest["argosy/argosy-mark-light.png"]
	if !present {
		t.Fatal("no latest check for the file")
	}
	if !got.OK || got.HTTPStatus == nil || *got.HTTPStatus != 200 {
		t.Errorf("latest = %+v, want the second (healthy) check", got)
	}
}

func TestStagedUploadRoundtrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	data := []byte("\x89PNG fake bytes")
	id, err := s.StageUpload(ctx, StagedUpload{
		Service: "amber", Filename: "amber-mark-light.png",
		ContentType: "image/png", SizeBytes: int64(len(data)),
		SHA256: "abc123", Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.StagedUploads(ctx, "amber")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != id || list[0].Data != nil {
		t.Fatalf("list = %+v, want one metadata-only row with id %s", list, id)
	}

	got, err := s.StagedData(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != string(data) || got.ContentType != "image/png" {
		t.Errorf("roundtrip lost the bytes: %+v", got)
	}

	if other, err := s.StagedUploads(ctx, "wiki"); err != nil || len(other) != 0 {
		t.Errorf("scoping leak: wiki sees %d uploads (err %v)", len(other), err)
	}
}
