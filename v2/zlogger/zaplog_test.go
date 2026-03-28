package zlogger_test

import (
	"context"
	"testing"
	"time"

	ormzap "github.com/gtkit/orm/v2/zlogger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type paramsFilteringLogger interface {
	ParamsFilter(ctx context.Context, sql string, params ...interface{}) (string, []interface{})
}

func TestLogModeSilentSuppressesSlowQueryLogs(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)

	logger := ormzap.New(ormzap.WithLogger(zap.New(core))).LogMode(gormlogger.Silent)
	logger.Trace(
		context.Background(),
		time.Now().Add(-time.Second),
		func() (string, int64) { return "SELECT 1", 1 },
		nil,
	)

	if entries := logs.All(); len(entries) != 0 {
		t.Fatalf("expected no log entries in silent mode, got %d", len(entries))
	}
}

func TestLogModeInfoLogsRegularQueries(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)

	logger := ormzap.New(ormzap.WithLogger(zap.New(core))).LogMode(gormlogger.Info)
	logger.Trace(
		context.Background(),
		time.Now().Add(-50*time.Millisecond),
		func() (string, int64) { return "SELECT 1", 1 },
		nil,
	)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}
	if entries[0].Level != zap.InfoLevel {
		t.Fatalf("expected info level, got %s", entries[0].Level)
	}
}

func TestIgnoreRecordNotFoundErrorSuppressesTrace(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)

	logger := ormzap.New(
		ormzap.WithLogger(zap.New(core)),
		ormzap.WithIgnoreRecordNotFoundError(true),
	)
	logger.Trace(
		context.Background(),
		time.Now().Add(-50*time.Millisecond),
		func() (string, int64) { return "SELECT 1", 0 },
		gorm.ErrRecordNotFound,
	)

	if entries := logs.All(); len(entries) != 0 {
		t.Fatalf("expected record-not-found trace to be suppressed, got %d entries", len(entries))
	}
}

func TestParameterizedQueriesHideParameters(t *testing.T) {
	filtering, ok := ormzap.New(
		ormzap.WithLogger(zap.NewNop()),
		ormzap.WithParameterizedQueries(true),
	).(paramsFilteringLogger)
	if !ok {
		t.Fatalf("expected logger to implement ParamsFilter")
	}

	sql, params := filtering.ParamsFilter(context.Background(), "SELECT * FROM users WHERE id = ?", 42)
	if sql != "SELECT * FROM users WHERE id = ?" {
		t.Fatalf("unexpected sql %q", sql)
	}
	if len(params) != 0 {
		t.Fatalf("expected params to be hidden, got %#v", params)
	}
}
