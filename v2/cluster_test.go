package orm

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestClusterReaderRoundRobin(t *testing.T) {
	primaryDB, _ := newStubDB()
	defer primaryDB.Close()
	replica1DB, _ := newStubDB()
	defer replica1DB.Close()
	replica2DB, _ := newStubDB()
	defer replica2DB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica1, err := OpenWithDB(context.Background(), replica1DB, WithName("replica-a"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica1: %v", err)
	}
	replica2, err := OpenWithDB(context.Background(), replica2DB, WithName("replica-b"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica2: %v", err)
	}

	cluster, err := NewCluster(primary, replica1, replica2)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	if got := cluster.Reader().Name(); got != "replica-a" {
		t.Fatalf("expected first reader replica-a, got %q", got)
	}
	if got := cluster.Reader().Name(); got != "replica-b" {
		t.Fatalf("expected second reader replica-b, got %q", got)
	}
	if got := cluster.Reader().Name(); got != "replica-a" {
		t.Fatalf("expected third reader replica-a, got %q", got)
	}
}

func TestClusterWithTxAndReadTx(t *testing.T) {
	primaryDB, primaryState := newStubDB()
	defer primaryDB.Close()
	replicaDB, replicaState := newStubDB()
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	if err := cluster.WithTx(context.Background(), func(tx *gorm.DB) error { return nil }); err != nil {
		t.Fatalf("cluster WithTx() error = %v", err)
	}
	if got := primaryState.beginCount.Load(); got != 1 {
		t.Fatalf("expected primary begin count 1, got %d", got)
	}
	if got := primaryState.commitCount.Load(); got != 1 {
		t.Fatalf("expected primary commit count 1, got %d", got)
	}

	if err := cluster.WithReadTx(context.Background(), func(tx *gorm.DB) error { return nil }); err != nil {
		t.Fatalf("cluster WithReadTx() error = %v", err)
	}
	if got := replicaState.readOnlyCount.Load(); got != 1 {
		t.Fatalf("expected replica read-only begin count 1, got %d", got)
	}
	if got := replicaState.commitCount.Load(); got != 1 {
		t.Fatalf("expected replica commit count 1, got %d", got)
	}
}

func TestDrainAndRecoverReplica(t *testing.T) {
	primaryDB, _ := newStubDB()
	defer primaryDB.Close()
	replicaDB, _ := newStubDB()
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	if err := cluster.DrainReplica("replica", errors.New("replication lag")); err != nil {
		t.Fatalf("DrainReplica() error = %v", err)
	}

	replicas := cluster.ReplicaNodes()
	if replicas[0].State() != NodeStateDraining {
		t.Fatalf("expected draining replica, got %q", replicas[0].State())
	}
	if got := cluster.Reader().Name(); got != "primary" {
		t.Fatalf("expected reads to fall back to primary, got %q", got)
	}

	if err := cluster.RecoverReplica(context.Background(), "replica"); err != nil {
		t.Fatalf("RecoverReplica() error = %v", err)
	}
	if replicas = cluster.ReplicaNodes(); replicas[0].State() != NodeStateReady {
		t.Fatalf("expected recovered replica ready, got %q", replicas[0].State())
	}
	if got := cluster.Reader().Name(); got != "replica" {
		t.Fatalf("expected reads to return to replica, got %q", got)
	}
}

func TestClusterHealthCheckDegradedWhenReplicaDown(t *testing.T) {
	primaryDB, primaryState := newStubDB()
	defer primaryDB.Close()
	replicaDB, replicaState := newStubDB(withStubPingError(errors.New("replica unavailable")))
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	report := cluster.HealthCheck(context.Background())
	if report.Status != HealthStatusDegraded {
		t.Fatalf("expected degraded cluster status, got %q", report.Status)
	}
	if len(report.Nodes) != 2 {
		t.Fatalf("expected 2 node reports, got %d", len(report.Nodes))
	}
	if got := primaryState.pingCount.Load(); got != 1 {
		t.Fatalf("expected primary health ping once, got %d", got)
	}
	if got := replicaState.pingCount.Load(); got != 1 {
		t.Fatalf("expected replica health ping once, got %d", got)
	}

	metrics := cluster.Metrics()
	if len(metrics) != 20 {
		t.Fatalf("expected 20 metric samples, got %d", len(metrics))
	}
}

func TestClusterRefreshAutomaticFailover(t *testing.T) {
	primaryDB, _ := newStubDB(withStubPingError(errors.New("primary unavailable")))
	defer primaryDB.Close()
	replicaDB, _ := newStubDB()
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica-a"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewClusterWithOptions(primary, []*Client{replica}, WithFailoverMode(FailoverAutomatic))
	if err != nil {
		t.Fatalf("NewClusterWithOptions() error = %v", err)
	}

	report, err := cluster.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if !report.FailedOver {
		t.Fatalf("expected failover to occur")
	}
	if report.PromotedTo != "replica-a" {
		t.Fatalf("expected promoted replica-a, got %q", report.PromotedTo)
	}
	if got := cluster.PrimaryNode().Name(); got != "replica-a" {
		t.Fatalf("expected new primary replica-a, got %q", got)
	}
	if cluster.PrimaryNode().Role() != RolePrimary {
		t.Fatalf("expected promoted node to be primary")
	}
	if report.Status != HealthStatusDegraded {
		t.Fatalf("expected degraded report after failover, got %q", report.Status)
	}

	replicas := cluster.ReplicaNodes()
	if len(replicas) != 1 || replicas[0].Name() != "primary" {
		t.Fatalf("expected old primary to become replica")
	}
	if replicas[0].State() != NodeStateDown {
		t.Fatalf("expected old primary to stay down, got %q", replicas[0].State())
	}
}

func TestClusterRefreshAutomaticFailoverWithoutReplicaFails(t *testing.T) {
	primaryDB, _ := newStubDB(withStubPingError(errors.New("primary unavailable")))
	defer primaryDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}

	cluster, err := NewClusterWithOptions(primary, nil, WithFailoverMode(FailoverAutomatic))
	if err != nil {
		t.Fatalf("NewClusterWithOptions() error = %v", err)
	}

	report, err := cluster.Refresh(context.Background())
	if !errors.Is(err, errNoFailoverTarget) {
		t.Fatalf("expected errNoFailoverTarget, got %v", err)
	}
	if report.Status != HealthStatusDown {
		t.Fatalf("expected down report, got %q", report.Status)
	}
}

func TestMarkPrimaryDownTriggersAutomaticFailover(t *testing.T) {
	primaryDB, _ := newStubDB()
	defer primaryDB.Close()
	replicaDB, _ := newStubDB()
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewClusterWithOptions(primary, []*Client{replica}, WithFailoverMode(FailoverAutomatic))
	if err != nil {
		t.Fatalf("NewClusterWithOptions() error = %v", err)
	}

	if err := cluster.MarkPrimaryDown(context.Background(), errors.New("write timeout")); err != nil {
		t.Fatalf("MarkPrimaryDown() error = %v", err)
	}
	if got := cluster.PrimaryNode().Name(); got != "replica" {
		t.Fatalf("expected replica to become primary, got %q", got)
	}
}

func TestClusterHealthCheckDownWhenPrimaryDown(t *testing.T) {
	primaryDB, _ := newStubDB(withStubPingError(errors.New("primary unavailable")))
	defer primaryDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}

	cluster, err := NewCluster(primary)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	report := cluster.HealthCheck(context.Background())
	if report.Status != HealthStatusDown {
		t.Fatalf("expected down cluster status, got %q", report.Status)
	}
	if report.Healthy() {
		t.Fatalf("expected unhealthy cluster report")
	}
}

func TestReaderClientWithoutFallbackReturnsError(t *testing.T) {
	primaryDB, _ := newStubDB()
	defer primaryDB.Close()
	replicaDB, _ := newStubDB()
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewClusterWithOptions(primary, []*Client{replica}, WithReadFallbackToPrimary(false))
	if err != nil {
		t.Fatalf("NewClusterWithOptions() error = %v", err)
	}

	if err := cluster.DrainReplica("replica", nil); err != nil {
		t.Fatalf("DrainReplica() error = %v", err)
	}

	client, err := cluster.ReaderClient()
	if !errors.Is(err, errNoReadableNode) {
		t.Fatalf("expected errNoReadableNode, got %v", err)
	}
	if client != nil {
		t.Fatalf("expected nil reader client when no readable node")
	}
}
