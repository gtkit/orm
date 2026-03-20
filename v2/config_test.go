package orm

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDriverConfigAndRedactedDSN(t *testing.T) {
	cfg := NewConfig(
		WithHost("db.internal"),
		WithPort("4406"),
		WithDatabase("app/main"),
		WithUser("alice"),
		WithPassword("secret"),
		WithTimeout(15*time.Second),
		WithReadTimeout(3*time.Second),
		WithWriteTimeout(4*time.Second),
		WithDSNParam("loc", "ignored-by-driver-config"),
	)

	driverCfg, err := cfg.DriverConfig()
	if err != nil {
		t.Fatalf("DriverConfig() error = %v", err)
	}

	if driverCfg.Addr != "db.internal:4406" {
		t.Fatalf("expected addr db.internal:4406, got %q", driverCfg.Addr)
	}
	if driverCfg.DBName != "app/main" {
		t.Fatalf("expected db name app/main, got %q", driverCfg.DBName)
	}
	if driverCfg.Timeout != 15*time.Second {
		t.Fatalf("expected timeout 15s, got %v", driverCfg.Timeout)
	}
	if driverCfg.ReadTimeout != 3*time.Second {
		t.Fatalf("expected read timeout 3s, got %v", driverCfg.ReadTimeout)
	}
	if driverCfg.WriteTimeout != 4*time.Second {
		t.Fatalf("expected write timeout 4s, got %v", driverCfg.WriteTimeout)
	}
	if got := driverCfg.Params["charset"]; got != "utf8mb4" {
		t.Fatalf("expected charset utf8mb4, got %q", got)
	}

	dsn, err := cfg.RedactedDSN()
	if err != nil {
		t.Fatalf("RedactedDSN() error = %v", err)
	}
	if strings.Contains(dsn, "secret") {
		t.Fatalf("expected redacted dsn to hide password, got %q", dsn)
	}
	if !strings.Contains(dsn, "/app%2Fmain") {
		t.Fatalf("expected database name to be escaped, got %q", dsn)
	}
}

func TestConfigCloneIsIsolated(t *testing.T) {
	base := DefaultConfig()
	clone := base.With(WithDSNParam("readPreference", "secondary"))
	clone.MySQL.Params["charset"] = "latin1"

	if got := base.MySQL.Params["charset"]; got != "utf8mb4" {
		t.Fatalf("expected original charset utf8mb4, got %q", got)
	}
	if _, ok := base.MySQL.Params["readPreference"]; ok {
		t.Fatalf("expected original params to stay isolated")
	}
}

func TestOpenWithDBUsesExternalPool(t *testing.T) {
	sqlDB, state := newStubDB()
	defer sqlDB.Close()

	cfg := NewConfig(
		WithMaxOpenConns(20),
		WithMaxIdleConns(8),
		WithConnMaxLifetime(time.Minute),
		WithConnMaxIdleTime(30*time.Second),
		WithSkipInitializeWithVersion(true),
	)

	client, err := cfg.OpenWithDB(context.Background(), sqlDB)
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	if client.DB() == nil {
		t.Fatalf("expected gorm db to be initialized")
	}
	if client.SQLDB() != sqlDB {
		t.Fatalf("expected wrapped sql.DB to be preserved")
	}

	stats := client.Stats()
	if stats.MaxOpenConnections != 20 {
		t.Fatalf("expected max open connections 20, got %d", stats.MaxOpenConnections)
	}

	if got := state.pingCount.Load(); got != 1 {
		t.Fatalf("expected startup ping once, got %d", got)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Fatalf("expected external sql.DB to remain open, got %v", err)
	}
	if got := state.pingCount.Load(); got != 2 {
		t.Fatalf("expected ping count 2 after manual ping, got %d", got)
	}
}

func TestOpenWithoutStartupPingDoesNotDialImmediately(t *testing.T) {
	client, err := Open(
		context.Background(),
		WithHost("127.0.0.1"),
		WithPort("1"),
		WithDatabase("app"),
		WithUser("root"),
		WithPassword("secret"),
		WithStartupPing(false),
		WithSkipInitializeWithVersion(true),
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	if client.DB() == nil || client.SQLDB() == nil {
		t.Fatalf("expected client to expose initialized db handles")
	}
}
