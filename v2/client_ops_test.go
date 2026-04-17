package orm

import (
	"context"
	"errors"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func TestClientWithTxCommitAndRollback(t *testing.T) {
	sqlDB, state := newStubDB()
	defer sqlDB.Close()

	client, err := OpenWithDB(
		context.Background(),
		sqlDB,
		WithName("orders"),
		WithStartupPing(false),
		WithSkipInitializeWithVersion(true),
		WithMaxOpenConns(12),
	)
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	if txErr := client.WithTx(context.Background(), nil, func(tx *gorm.DB) error {
		if tx == nil {
			t.Fatalf("expected tx db")
		}
		return nil
	}); txErr != nil {
		t.Fatalf("WithTx() error = %v", txErr)
	}

	if got := state.beginCount.Load(); got != 1 {
		t.Fatalf("expected begin count 1, got %d", got)
	}
	if got := state.commitCount.Load(); got != 1 {
		t.Fatalf("expected commit count 1, got %d", got)
	}
	if got := state.rollbackCount.Load(); got != 0 {
		t.Fatalf("expected rollback count 0, got %d", got)
	}

	wantErr := errors.New("boom")
	err = client.WithReadTx(context.Background(), func(tx *gorm.DB) error {
		if tx == nil {
			t.Fatalf("expected tx db")
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if got := state.readOnlyCount.Load(); got != 1 {
		t.Fatalf("expected read-only begin count 1, got %d", got)
	}
	if got := state.rollbackCount.Load(); got != 1 {
		t.Fatalf("expected rollback count 1, got %d", got)
	}
}

func TestClientHealthCheckAndMetrics(t *testing.T) {
	sqlDB, state := newStubDB()
	defer sqlDB.Close()

	client, err := OpenWithDB(
		context.Background(),
		sqlDB,
		WithName("orders-primary"),
		WithStartupPing(false),
		WithSkipInitializeWithVersion(true),
		WithMaxOpenConns(20),
		WithMaxIdleConns(5),
	)
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	report := client.HealthCheck(context.Background())
	if report.Name != "orders-primary" {
		t.Fatalf("expected report name orders-primary, got %q", report.Name)
	}
	if report.Role != RoleStandalone {
		t.Fatalf("expected standalone role, got %q", report.Role)
	}
	if report.Status != HealthStatusUp {
		t.Fatalf("expected health up, got %q", report.Status)
	}
	if report.Duration < 0 {
		t.Fatalf("expected non-negative duration, got %v", report.Duration)
	}
	if got := state.pingCount.Load(); got != 1 {
		t.Fatalf("expected one health-check ping, got %d", got)
	}
	if report.Stats.MaxOpenConnections != 20 {
		t.Fatalf("expected max open connections 20, got %d", report.Stats.MaxOpenConnections)
	}

	metrics := client.Metrics()
	if len(metrics) != 10 {
		t.Fatalf("expected 10 metric samples, got %d", len(metrics))
	}
	if metrics[0].Labels["name"] != "orders-primary" {
		t.Fatalf("expected metric label name orders-primary, got %q", metrics[0].Labels["name"])
	}
	if metrics[0].Labels["role"] != string(RoleStandalone) {
		t.Fatalf("expected role label standalone, got %q", metrics[0].Labels["role"])
	}
}

func TestClientHealthCheckDown(t *testing.T) {
	sqlDB, _ := newStubDB(withStubPingError(errors.New("dial timeout")))
	defer sqlDB.Close()

	client, err := OpenWithDB(
		context.Background(),
		sqlDB,
		WithStartupPing(false),
		WithSkipInitializeWithVersion(true),
	)
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	report := client.HealthCheck(context.Background())
	if report.Status != HealthStatusDown {
		t.Fatalf("expected health down, got %q", report.Status)
	}
	if report.Error == nil {
		t.Fatalf("expected health error")
	}
}

func TestClientHealthCheckAcceptsNilContext(t *testing.T) {
	sqlDB, state := newStubDB()
	defer sqlDB.Close()

	client, err := OpenWithDB(
		context.Background(),
		sqlDB,
		WithStartupPing(false),
		WithSkipInitializeWithVersion(true),
	)
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	report := client.HealthCheck(nil)
	if report.Status != HealthStatusUp {
		t.Fatalf("expected health up, got %q", report.Status)
	}
	if got := state.pingCount.Load(); got != 1 {
		t.Fatalf("expected one health-check ping, got %d", got)
	}
}

func TestWithTxRetriesOnDeadlock(t *testing.T) {
	mysqlDeadlock := &mysqldriver.MySQLError{Number: mysqlErrDeadlock, Message: "Deadlock found"}

	sqlDB, state := newStubDB()
	state.commitErrOnce = mysqlDeadlock // first commit fails with deadlock
	defer sqlDB.Close()

	client, err := OpenWithDB(context.Background(), sqlDB,
		WithName("retry-test"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	callCount := 0
	txErr := client.WithTx(context.Background(), nil, func(_ *gorm.DB) error {
		callCount++
		return nil
	})
	if txErr != nil {
		t.Fatalf("WithTx() expected success after retry, got %v", txErr)
	}
	// fn called twice: first attempt deadlocks on commit, second succeeds.
	if callCount != 2 {
		t.Fatalf("expected fn called 2 times (1 deadlock + 1 success), got %d", callCount)
	}
	// 2 begins, 2 commits (1 failed + 1 success), 1 rollback from deadlock.
	if got := state.beginCount.Load(); got != 2 {
		t.Fatalf("expected begin count 2, got %d", got)
	}
	if got := state.commitCount.Load(); got != 2 {
		t.Fatalf("expected commit count 2, got %d", got)
	}
}

func TestWithTxNoRetryOnNonDeadlock(t *testing.T) {
	sqlDB, _ := newStubDB()
	defer sqlDB.Close()

	client, err := OpenWithDB(context.Background(), sqlDB,
		WithName("no-retry"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	businessErr := errors.New("validation failed")
	callCount := 0
	txErr := client.WithTx(context.Background(), nil, func(_ *gorm.DB) error {
		callCount++
		return businessErr
	})
	if !errors.Is(txErr, businessErr) {
		t.Fatalf("expected business error, got %v", txErr)
	}
	if callCount != 1 {
		t.Fatalf("expected fn called once (no retry for non-deadlock), got %d", callCount)
	}
}

func TestWithTxRetriesExhausted(t *testing.T) {
	mysqlDeadlock := &mysqldriver.MySQLError{Number: mysqlErrDeadlock, Message: "Deadlock found"}

	sqlDB, state := newStubDB()
	state.commitErr = mysqlDeadlock // every commit fails
	defer sqlDB.Close()

	client, err := OpenWithDB(context.Background(), sqlDB,
		WithName("exhaust"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	callCount := 0
	txErr := client.WithTx(context.Background(), nil, func(_ *gorm.DB) error {
		callCount++
		return nil
	}, WithMaxRetries(2), WithRetryBaseWait(time.Millisecond))

	if txErr == nil {
		t.Fatal("expected error after retries exhausted")
	}
	// 1 initial + 2 retries = 3 calls.
	if callCount != 3 {
		t.Fatalf("expected fn called 3 times, got %d", callCount)
	}
	if got := state.beginCount.Load(); got != 3 {
		t.Fatalf("expected begin count 3, got %d", got)
	}
}

func TestWithTxZeroRetriesDisablesRetry(t *testing.T) {
	mysqlDeadlock := &mysqldriver.MySQLError{Number: mysqlErrDeadlock, Message: "Deadlock found"}

	sqlDB, state := newStubDB()
	state.commitErr = mysqlDeadlock
	defer sqlDB.Close()

	client, err := OpenWithDB(context.Background(), sqlDB,
		WithName("no-retry"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("OpenWithDB() error = %v", err)
	}

	callCount := 0
	txErr := client.WithTx(context.Background(), nil, func(_ *gorm.DB) error {
		callCount++
		return nil
	}, WithMaxRetries(0))

	if txErr == nil {
		t.Fatal("expected deadlock error")
	}
	if callCount != 1 {
		t.Fatalf("expected fn called once with retries disabled, got %d", callCount)
	}
	if got := state.beginCount.Load(); got != 1 {
		t.Fatalf("expected begin count 1, got %d", got)
	}
}

func TestDBStatsSnapshotUtilization(t *testing.T) {
	snapshot := DBStatsSnapshot{
		MaxOpenConnections: 10,
		InUse:              4,
		WaitDuration:       time.Second,
	}
	snapshot.Utilization = float64(snapshot.InUse) / float64(snapshot.MaxOpenConnections)

	metrics := snapshot.metrics(metricLabels("orders", RolePrimary))
	if metrics[9].Value != 0.4 {
		t.Fatalf("expected utilization 0.4, got %v", metrics[9].Value)
	}
}
