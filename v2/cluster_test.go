package orm

import (
	"context"
	"errors"
	"sync"
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

	if txErr := cluster.WithTx(context.Background(), func(_ *gorm.DB) error { return nil }); txErr != nil {
		t.Fatalf("cluster WithTx() error = %v", txErr)
	}
	if got := primaryState.beginCount.Load(); got != 1 {
		t.Fatalf("expected primary begin count 1, got %d", got)
	}
	if got := primaryState.commitCount.Load(); got != 1 {
		t.Fatalf("expected primary commit count 1, got %d", got)
	}

	if txErr := cluster.WithReadTx(context.Background(), func(_ *gorm.DB) error { return nil }); txErr != nil {
		t.Fatalf("cluster WithReadTx() error = %v", txErr)
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

	if drainErr := cluster.DrainReplica("replica", errors.New("replication lag")); drainErr != nil {
		t.Fatalf("DrainReplica() error = %v", drainErr)
	}

	replicas := cluster.ReplicaNodes()
	if replicas[0].State() != NodeStateDraining {
		t.Fatalf("expected draining replica, got %q", replicas[0].State())
	}
	if got := cluster.Reader().Name(); got != "primary" {
		t.Fatalf("expected reads to fall back to primary, got %q", got)
	}

	if recoverErr := cluster.RecoverReplica(context.Background(), "replica"); recoverErr != nil {
		t.Fatalf("RecoverReplica() error = %v", recoverErr)
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

func TestClusterRefreshMarksPrimaryDownWithoutSwitchingTopology(t *testing.T) {
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

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	report := cluster.Refresh(context.Background())
	if report.Status != HealthStatusDown {
		t.Fatalf("expected down report, got %q", report.Status)
	}
	if got := cluster.PrimaryNode().Name(); got != "primary" {
		t.Fatalf("expected primary routing to stay on primary, got %q", got)
	}
	if cluster.PrimaryNode().State() != NodeStateDown {
		t.Fatalf("expected primary node state down, got %q", cluster.PrimaryNode().State())
	}
	if got := cluster.Reader().Name(); got != "replica-a" {
		t.Fatalf("expected reads to keep using healthy replica, got %q", got)
	}
}

func TestSwitchPrimaryUpdatesWriteRoutingOnlyAfterExplicitOperatorAction(t *testing.T) {
	primaryDB, _ := newStubDB()
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

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	switched, err := cluster.SwitchPrimary(context.Background(), "replica-a")
	if err != nil {
		t.Fatalf("SwitchPrimary() error = %v", err)
	}
	if switched.Name() != "replica-a" {
		t.Fatalf("expected switched node replica-a, got %q", switched.Name())
	}
	if got := cluster.PrimaryNode().Name(); got != "replica-a" {
		t.Fatalf("expected current primary replica-a, got %q", got)
	}
	if got := cluster.WriteDB(); got == nil || cluster.Primary().Name() != "replica-a" {
		t.Fatalf("expected writes to route to replica-a after explicit switch")
	}

	replicas := cluster.ReplicaNodes()
	if len(replicas) != 1 || replicas[0].Name() != "primary" {
		t.Fatalf("expected previous primary to move into replica set")
	}
	if replicas[0].State() != NodeStateDraining {
		t.Fatalf("expected previous primary to drain until explicitly recovered, got %q", replicas[0].State())
	}
}

func TestSwitchPrimaryReturnsPrimaryWhenConcurrentSwitchAlreadyWon(t *testing.T) {
	primaryDB, _ := newStubDB()
	defer primaryDB.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	replicaDB, _ := newStubDB(withStubPingHook(func() {
		once.Do(func() {
			close(started)
			<-release
		})
	}))
	defer replicaDB.Close()

	primary, err := OpenWithDB(context.Background(), primaryDB, WithName("primary"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open primary: %v", err)
	}
	replica, err := OpenWithDB(context.Background(), replicaDB, WithName("replica-a"), WithStartupPing(false), WithSkipInitializeWithVersion(true))
	if err != nil {
		t.Fatalf("open replica: %v", err)
	}

	cluster, err := NewCluster(primary, replica)
	if err != nil {
		t.Fatalf("NewCluster() error = %v", err)
	}

	type switchResult struct {
		node Node
		err  error
	}
	resultCh := make(chan switchResult, 1)
	go func() {
		node, switchErr := cluster.SwitchPrimary(context.Background(), "replica-a")
		resultCh <- switchResult{node: node, err: switchErr}
	}()

	<-started

	cluster.mu.Lock()
	concurrentCandidate := cluster.findReplicaLocked("replica-a")
	if concurrentCandidate == nil {
		cluster.mu.Unlock()
		t.Fatalf("expected replica-a to still be a replica before concurrent switch")
	}
	cluster.switchPrimaryLocked(concurrentCandidate, errors.New("concurrent switch"))
	cluster.mu.Unlock()

	close(release)

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("expected concurrent loser to observe current primary, got error %v", result.err)
	}
	if result.node.Name() != "replica-a" {
		t.Fatalf("expected returned node replica-a, got %q", result.node.Name())
	}
	if cluster.PrimaryNode().Name() != "replica-a" {
		t.Fatalf("expected cluster primary replica-a, got %q", cluster.PrimaryNode().Name())
	}
}

func TestMarkPrimaryDownStopsWritesWithoutAutomaticPromotion(t *testing.T) {
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

	if markErr := cluster.MarkPrimaryDown(errors.New("write timeout")); markErr != nil {
		t.Fatalf("MarkPrimaryDown() error = %v", markErr)
	}

	writer, err := cluster.WriteClient()
	if !errors.Is(err, errPrimaryUnavailable) {
		t.Fatalf("expected errPrimaryUnavailable, got %v", err)
	}
	if writer != nil {
		t.Fatalf("expected nil writer when primary is down")
	}
	if got := cluster.PrimaryNode().Name(); got != "primary" {
		t.Fatalf("expected topology to remain on primary, got %q", got)
	}
	if got := cluster.Reader().Name(); got != "replica" {
		t.Fatalf("expected reads to keep using replica, got %q", got)
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

	if drainErr := cluster.DrainReplica("replica", nil); drainErr != nil {
		t.Fatalf("DrainReplica() error = %v", drainErr)
	}

	client, err := cluster.ReaderClient()
	if !errors.Is(err, errNoReadableNode) {
		t.Fatalf("expected errNoReadableNode, got %v", err)
	}
	if client != nil {
		t.Fatalf("expected nil reader client when no readable node")
	}
}
