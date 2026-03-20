package zlogger

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestTraceLogsRecordNotFoundAsWarning(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)

	logger, ok := New(WithLogger(zap.New(core))).(GormLogger)
	if !ok {
		t.Fatalf("expected GormLogger")
	}

	logger.Trace(
		context.Background(),
		time.Now().Add(-100*time.Millisecond),
		func() (string, int64) { return "SELECT 1", 0 },
		gorm.ErrRecordNotFound,
	)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one log entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Level != zap.WarnLevel {
		t.Fatalf("expected warn level, got %s", entry.Level)
	}
	if entry.Message != "Database ErrRecordNotFound" {
		t.Fatalf("unexpected message %q", entry.Message)
	}
}
