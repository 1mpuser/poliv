package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFS embed.FS

type Migrator struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool}
}

// Up — поднимает миграции до head. На существующей (Alembic) базе схема уже есть —
// базовая миграция пропускается, применяются только более поздние. На пустой — создаёт
// полную схему, идентичную Alembic head (0001–0005).
func (m *Migrator) Up(ctx context.Context) error {
	if _, err := m.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS pg_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	files, err := fs.Glob(sqlFS, "sql/*.sql")
	if err != nil {
		return err
	}
	applied, err := m.appliedSet(ctx)
	if err != nil {
		return err
	}
	// Определяем, есть ли уже схема (Alembic). Если есть — базовую не применяем,
	// но помечаем её как применённую, чтобы следующие миграции пошли по номерам.
	if !m.schemaExists(ctx) {
		// пустая база — применяем все с нуля
	} else {
		// схема от Alembic: базовая считается применённой, данные не трогаем
		if _, ok := applied["0001_baseline"]; !ok {
			if err := m.record(ctx, "0001_baseline"); err != nil {
				return err
			}
		}
	}

	sort.Strings(files)
	for _, f := range files {
		name := strings.TrimSuffix(path.Base(f), ".sql")
		if _, ok := applied[name]; ok {
			continue
		}
		sqlBytes, err := sqlFS.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := m.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err := m.record(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) schemaExists(ctx context.Context) bool {
	var ok bool
	err := m.pool.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&ok)
	if err != nil {
		return false
	}
	return ok
}

func (m *Migrator) appliedSet(ctx context.Context) (map[string]bool, error) {
	rows, err := m.pool.Query(ctx, `SELECT version FROM pg_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		set[v] = true
	}
	return set, rows.Err()
}

func (m *Migrator) record(ctx context.Context, version string) error {
	_, err := m.pool.Exec(ctx, `INSERT INTO pg_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`, version)
	return err
}
