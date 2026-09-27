package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
	Zone *time.Location
}

func New(ctx context.Context, url string, zone *time.Location) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{Pool: pool, Zone: zone}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

// NowUTC — как now_utc() в Python: текущее UTC-время.
func NowUTC() time.Time {
	return time.Now().UTC()
}
