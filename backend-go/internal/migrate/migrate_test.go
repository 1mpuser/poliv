package migrate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestUpOnExistingSchema — база с уже существующей схемой (как прод после Alembic):
// базовая миграция не применяется, Up завершается без ошибок и идемпотентен.
// Баги-регрессия: applied-множество снималось до отметки baseline, и baseline
// применялся повторно по SQL — «type already exists» при старте на прод-базе.
func TestUpOnExistingSchema(t *testing.T) {
	srcURL := os.Getenv("TEST_DATABASE_URL")
	if srcURL == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	src, err := url.Parse(srcURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	admin, err := pgxpool.New(context.Background(), srcURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()

	scratch := fmt.Sprintf("poliv_migrate_test_%d", os.Getpid())
	if _, err := admin.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %s`, scratch)); err != nil {
		t.Fatalf("create scratch db: %v", err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE %s WITH (FORCE)`, scratch)); err != nil {
			t.Logf("drop scratch db: %v", err)
		}
	}()

	scratchURL := *src
	scratchURL.Path = "/" + scratch
	pool, err := pgxpool.New(context.Background(), scratchURL.String())
	if err != nil {
		t.Fatalf("connect scratch: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()

	// Существующая схема: полная baseline, как её оставил Alembic на проде.
	baseline, err := sqlFS.ReadFile("sql/0001_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if _, err := pool.Exec(ctx, string(baseline)); err != nil {
		t.Fatalf("apply existing schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE alembic_version (version_num varchar(32) NOT NULL, CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num))`); err != nil {
		t.Fatalf("create alembic_version: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO alembic_version VALUES ('0005')`); err != nil {
		t.Fatalf("insert alembic_version: %v", err)
	}

	if err := New(pool).Up(ctx); err != nil {
		t.Fatalf("Up на существующей схеме: %v", err)
	}

	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_migrations WHERE version = '0001_baseline'`).Scan(&got); err != nil {
		t.Fatalf("check pg_migrations: %v", err)
	}
	if got != 1 {
		t.Fatalf("0001_baseline в pg_migrations: %d, ждём 1", got)
	}

	// Идемпотентность: повторный старт (рестарт контейнера) тоже без ошибок.
	if err := New(pool).Up(ctx); err != nil {
		t.Fatalf("повторный Up: %v", err)
	}

	// Схема не изменилась: alembic_version на месте, pg_migrations — единственное новое.
	var alembic string
	if err := pool.QueryRow(ctx, `SELECT version_num FROM alembic_version`).Scan(&alembic); err != nil || alembic != "0005" {
		t.Fatalf("alembic_version после Up: %q (err %v), ждём 0005", alembic, err)
	}
}
