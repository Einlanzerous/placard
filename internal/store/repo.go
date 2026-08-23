package store

import (
	"context"
	"time"
)

// Check kinds. Canonical rows verify the jsDelivr URLs the launcher is given;
// edge rows verify placard's OWN public hostname stays sessionless-fetchable
// (PCAD-10 — an Access gate there is invisible to everyone holding a session).
const (
	KindCanonical = "canonical"
	KindEdge      = "edge"
)

// MarkCheck is one verification of one URL.
type MarkCheck struct {
	Kind          string     `json:"-"`
	Service       string     `json:"-"`
	File          string     `json:"-"`
	URL           string     `json:"url"`
	OK            bool       `json:"ok"`
	HTTPStatus    *int       `json:"http_status"`
	ContentType   *string    `json:"content_type"`
	ContentLength *int64     `json:"content_length"`
	Error         *string    `json:"error"`
	CheckedAt     time.Time  `json:"checked_at"`
}

func (s *Store) RecordCheck(ctx context.Context, c MarkCheck) error {
	if c.Kind == "" {
		c.Kind = KindCanonical
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mark_check (kind, service, file, url, ok, http_status, content_type, content_length, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.Kind, c.Service, c.File, c.URL, c.OK, c.HTTPStatus, c.ContentType, c.ContentLength, c.Error)
	return err
}

// LatestChecks returns the most recent check of the given kind per
// repo-relative file path.
func (s *Store) LatestChecks(ctx context.Context, kind string) (map[string]MarkCheck, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (service, file)
		       kind, service, file, url, ok, http_status, content_type, content_length, error, checked_at
		FROM mark_check
		WHERE kind = $1
		ORDER BY service, file, checked_at DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]MarkCheck)
	for rows.Next() {
		var c MarkCheck
		if err := rows.Scan(&c.Kind, &c.Service, &c.File, &c.URL, &c.OK, &c.HTTPStatus,
			&c.ContentType, &c.ContentLength, &c.Error, &c.CheckedAt); err != nil {
			return nil, err
		}
		out[c.File] = c
	}
	return out, rows.Err()
}

// StagedUpload is a mark dropped on the front page, awaiting a human commit.
type StagedUpload struct {
	ID          string    `json:"id"`
	Service     string    `json:"service"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	SHA256      string    `json:"sha256"`
	Data        []byte    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) StageUpload(ctx context.Context, up StagedUpload) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO staged_upload (service, filename, content_type, size_bytes, sha256, data)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		up.Service, up.Filename, up.ContentType, up.SizeBytes, up.SHA256, up.Data).Scan(&id)
	return id, err
}

// StagedUploads lists metadata (no bytes) newest-first, optionally scoped to
// one service ("" for all).
func (s *Store) StagedUploads(ctx context.Context, service string) ([]StagedUpload, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, service, filename, content_type, size_bytes, sha256, created_at
		FROM staged_upload
		WHERE ($1 = '' OR service = $1)
		ORDER BY created_at DESC`, service)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StagedUpload
	for rows.Next() {
		var up StagedUpload
		if err := rows.Scan(&up.ID, &up.Service, &up.Filename, &up.ContentType,
			&up.SizeBytes, &up.SHA256, &up.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, up)
	}
	return out, rows.Err()
}

// StagedData fetches one staged upload with its bytes.
func (s *Store) StagedData(ctx context.Context, id string) (StagedUpload, error) {
	var up StagedUpload
	err := s.pool.QueryRow(ctx, `
		SELECT id, service, filename, content_type, size_bytes, sha256, data, created_at
		FROM staged_upload WHERE id = $1`, id).
		Scan(&up.ID, &up.Service, &up.Filename, &up.ContentType,
			&up.SizeBytes, &up.SHA256, &up.Data, &up.CreatedAt)
	return up, err
}
