// Package dbtest gives database tests a private, freshly migrated Postgres
// database. Use it for tests that rewrite shared tables (TRUNCATE, global
// deletes): on the shared TEST_DATABASE_URL database such statements wipe the
// rows other packages' tests are using at the same time (go test runs
// packages in parallel). A TRUNCATE ... CASCADE of the catalog, for example,
// also empties vehicles and every table that references them.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FreshDatabase creates a new database on the server of baseDSN, applies
// every up migration in backend/migrations and returns its DSN. The database
// is dropped when the test ends. The role of baseDSN needs CREATEDB.
func FreshDatabase(t testing.TB, baseDSN string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("dbtest: parse dsn: %v", err)
	}
	base := strings.TrimPrefix(u.Path, "/")
	if base == "" {
		base = "postgres"
	}
	name := fmt.Sprintf("%s_t_%s", base, strings.ReplaceAll(uuid.NewString()[:13], "-", ""))
	if len(name) > 63 {
		name = name[len(name)-63:]
	}
	admin, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		t.Fatalf("dbtest: connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), baseDSN)
		if err != nil {
			t.Logf("dbtest: drop %s: %v", name, err)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		if _, err := c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Logf("dbtest: drop %s: %v", name, err)
		}
	})

	fresh := *u
	fresh.Path = "/" + name
	dsn := fresh.String()
	if err := migrate(ctx, dsn); err != nil {
		t.Fatalf("dbtest: migrate %s: %v", name, err)
	}
	return dsn
}

// migrate applies the up migrations in version order, like golang-migrate
// (one simple-protocol Exec per file).
func migrate(ctx context.Context, dsn string) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no migrations in %s", dir)
	}
	sort.Strings(files)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

func migrationsDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("dbtest: cannot locate source file")
	}
	// internal/database/dbtest/dbtest.go → backend/migrations
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"), nil
}
