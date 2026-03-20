package orm

import (
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestMysqlConfigBuildsDriverDSN(t *testing.T) {
	MysqlConfig(
		DbType("mariadb"),
		Host("db.internal"),
		Port("4406"),
		Name("app/main"),
		User("alice"),
		WithPassword("p@ss:word"),
		MaxOpenConns(25),
		MaxIdleConns(5),
		ConnMaxLifetime(30*time.Minute),
		ConnMaxIdleTime(10*time.Minute),
	)

	opts := mysqlOptionsSnapshot()
	if opts.DbType != "mariadb" {
		t.Fatalf("expected db type to be updated, got %q", opts.DbType)
	}
	if !opts.hasMaxOpenConns || opts.maxOpenConns != 25 {
		t.Fatalf("expected max open conns to be set, got %#v", opts)
	}
	if !opts.hasMaxIdleConns || opts.maxIdleConns != 5 {
		t.Fatalf("expected max idle conns to be set, got %#v", opts)
	}
	if !opts.hasConnMaxLifetime || opts.connMaxLifetime != 30*time.Minute {
		t.Fatalf("expected conn max lifetime to be set, got %#v", opts)
	}
	if !opts.hasConnMaxIdleTime || opts.connMaxIdleTime != 10*time.Minute {
		t.Fatalf("expected conn max idle time to be set, got %#v", opts)
	}

	dsn := new(Mysql).GetConnect()
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if cfg.User != "alice" {
		t.Fatalf("expected user alice, got %q", cfg.User)
	}
	if cfg.Passwd != "p@ss:word" {
		t.Fatalf("expected password to round-trip, got %q", cfg.Passwd)
	}
	if cfg.Addr != "db.internal:4406" {
		t.Fatalf("expected addr db.internal:4406, got %q", cfg.Addr)
	}
	if cfg.DBName != "app/main" {
		t.Fatalf("expected db name app/main, got %q", cfg.DBName)
	}
	if !cfg.ParseTime {
		t.Fatalf("expected parseTime to be enabled")
	}
	if cfg.Timeout != defaultDialTimeout {
		t.Fatalf("expected timeout %v, got %v", defaultDialTimeout, cfg.Timeout)
	}
	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Fatalf("expected dsn to include charset, got %q", dsn)
	}
	if !strings.Contains(dsn, "/app%2Fmain") {
		t.Fatalf("expected db name to be path-escaped, got %q", dsn)
	}
	if cfg.Loc == nil || cfg.Loc.String() != time.Local.String() {
		t.Fatalf("expected local location, got %v", cfg.Loc)
	}
}

func TestGormConfigResetsToDefaults(t *testing.T) {
	GormConfig(
		PrepareStmt(true),
		SkipDefaultTransaction(true),
		TablePrefix("t_"),
		SingularTable(true),
	)

	cfg := gormConfigSnapshot()
	if !cfg.PrepareStmt {
		t.Fatalf("expected prepare stmt to be enabled")
	}
	if !cfg.SkipDefaultTransaction {
		t.Fatalf("expected skip default transaction to be enabled")
	}

	strategy := currentNamingStrategy(&cfg)
	if strategy.TablePrefix != "t_" {
		t.Fatalf("expected table prefix t_, got %q", strategy.TablePrefix)
	}
	if !strategy.SingularTable {
		t.Fatalf("expected singular table to be enabled")
	}

	GormConfig()

	cfg = gormConfigSnapshot()
	if cfg.PrepareStmt {
		t.Fatalf("expected prepare stmt to reset to default")
	}
	if cfg.SkipDefaultTransaction {
		t.Fatalf("expected skip default transaction to reset to default")
	}

	strategy = currentNamingStrategy(&cfg)
	if strategy.TablePrefix != "" {
		t.Fatalf("expected empty table prefix after reset, got %q", strategy.TablePrefix)
	}
	if strategy.SingularTable {
		t.Fatalf("expected singular table to reset to false")
	}
}
