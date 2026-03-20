package orm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

var (
	errNilPrimaryClient   = errors.New("orm/v2: nil primary client")
	errNilReplicaClient   = errors.New("orm/v2: nil replica client")
	errReplicaNotFound    = errors.New("orm/v2: replica not found")
	errNoReadableNode     = errors.New("orm/v2: no readable node available")
	errPrimaryUnavailable = errors.New("orm/v2: primary unavailable")
	errNoFailoverTarget   = errors.New("orm/v2: no replica available for failover")
	errDuplicateNodeName  = errors.New("orm/v2: duplicate node name")
)

type FailoverMode string

const (
	FailoverManual    FailoverMode = "manual"
	FailoverAutomatic FailoverMode = "automatic"
)

type ClusterOption func(*clusterOptions)

type clusterOptions struct {
	failoverMode          FailoverMode
	readFallbackToPrimary bool
	autoRecoverReplicas   bool
}

type Cluster struct {
	mu          sync.RWMutex
	primary     *managedNode
	replicas    []*managedNode
	readerIndex atomic.Uint64
	options     clusterOptions
}

type managedNode struct {
	name      string
	role      NodeRole
	client    *Client
	state     NodeState
	lastError error
	updatedAt time.Time
}

type Node struct {
	name      string
	role      NodeRole
	client    *Client
	state     NodeState
	lastError error
	updatedAt time.Time
}

type ClusterHealthReport struct {
	Status     HealthStatus
	CheckedAt  time.Time
	Nodes      []HealthReport
	FailedOver bool
	PromotedTo string
}

func (r ClusterHealthReport) Healthy() bool {
	return r.Status == HealthStatusUp
}

func OpenCluster(ctx context.Context, primary Config, replicas ...Config) (*Cluster, error) {
	return OpenClusterWithOptions(ctx, primary, replicas)
}

func OpenClusterWithOptions(ctx context.Context, primary Config, replicas []Config, opts ...ClusterOption) (_ *Cluster, err error) {
	primaryClient, err := primary.Open(ctx)
	if err != nil {
		return nil, err
	}

	opened := []*Client{primaryClient}
	defer func() {
		if err == nil {
			return
		}
		for _, client := range opened {
			_ = client.Close()
		}
	}()

	replicaClients := make([]*Client, 0, len(replicas))
	for _, replica := range replicas {
		client, openErr := replica.Open(ctx)
		if openErr != nil {
			err = openErr
			return nil, err
		}
		opened = append(opened, client)
		replicaClients = append(replicaClients, client)
	}

	return NewClusterWithOptions(primaryClient, replicaClients, opts...)
}

func NewCluster(primary *Client, replicas ...*Client) (*Cluster, error) {
	return NewClusterWithOptions(primary, replicas)
}

func NewClusterWithOptions(primary *Client, replicas []*Client, opts ...ClusterOption) (*Cluster, error) {
	if primary == nil {
		return nil, errNilPrimaryClient
	}

	options := defaultClusterOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	cluster := &Cluster{
		primary: newManagedNode(primary.effectiveName("primary"), RolePrimary, primary),
		options: options,
	}

	usedNames := map[string]struct{}{
		cluster.primary.name: {},
	}

	cluster.replicas = make([]*managedNode, 0, len(replicas))
	for i, replica := range replicas {
		if replica == nil {
			return nil, errNilReplicaClient
		}

		name := replica.effectiveName(replicaName(i))
		if _, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("%w: %s", errDuplicateNodeName, name)
		}
		usedNames[name] = struct{}{}
		cluster.replicas = append(cluster.replicas, newManagedNode(name, RoleReplica, replica))
	}

	return cluster, nil
}

func WithFailoverMode(mode FailoverMode) ClusterOption {
	return func(options *clusterOptions) {
		if mode == "" {
			mode = FailoverManual
		}
		options.failoverMode = mode
	}
}

func WithReadFallbackToPrimary(enabled bool) ClusterOption {
	return func(options *clusterOptions) {
		options.readFallbackToPrimary = enabled
	}
}

func WithAutoRecoverReplicas(enabled bool) ClusterOption {
	return func(options *clusterOptions) {
		options.autoRecoverReplicas = enabled
	}
}

func (c *Cluster) Primary() *Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.primary == nil {
		return nil
	}
	return c.primary.client
}

func (c *Cluster) PrimaryNode() Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.primary == nil {
		return Node{}
	}
	return c.primary.snapshot()
}

func (c *Cluster) ReplicaNodes() []Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return snapshots(c.replicas)
}

func (c *Cluster) Nodes() []Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nodes := make([]Node, 0, 1+len(c.replicas))
	if c.primary != nil {
		nodes = append(nodes, c.primary.snapshot())
	}
	nodes = append(nodes, snapshots(c.replicas)...)
	return nodes
}

func (c *Cluster) Reader() *Client {
	client, _ := c.ReaderClient()
	return client
}

func (c *Cluster) ReaderClient() (*Client, error) {
	c.mu.RLock()
	candidates := c.readyReplicasLocked()
	primary := c.primary
	readFallback := c.options.readFallbackToPrimary
	c.mu.RUnlock()

	if len(candidates) > 0 {
		idx := c.readerIndex.Add(1) - 1
		return candidates[idx%uint64(len(candidates))].client, nil
	}
	if readFallback && primary != nil && primary.state == NodeStateReady {
		return primary.client, nil
	}
	return nil, errNoReadableNode
}

func (c *Cluster) WriteClient() (*Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.primary == nil || c.primary.state == NodeStateDown {
		return nil, errPrimaryUnavailable
	}
	return c.primary.client, nil
}

func (c *Cluster) WriteDB() *gorm.DB {
	client, _ := c.WriteClient()
	if client == nil {
		return nil
	}
	return client.DB()
}

func (c *Cluster) ReadDB() *gorm.DB {
	client, _ := c.ReaderClient()
	if client == nil {
		return nil
	}
	return client.DB()
}

func (c *Cluster) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	client, err := c.WriteClient()
	if err != nil {
		return err
	}
	return client.WithTx(ctx, nil, fn)
}

func (c *Cluster) WithReadTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	client, err := c.ReaderClient()
	if err != nil {
		return err
	}
	return client.WithReadTx(ctx, fn)
}

func (c *Cluster) HealthCheck(ctx context.Context) ClusterHealthReport {
	checkedAt := time.Now()
	nodes := c.Nodes()
	reports := probeNodes(ctx, nodes)
	return buildClusterReport(checkedAt, reports, false, "")
}

func (c *Cluster) Refresh(ctx context.Context) (ClusterHealthReport, error) {
	checkedAt := time.Now()
	nodes := c.Nodes()
	probed := probeNodesByName(ctx, nodes)

	c.mu.Lock()
	for _, node := range c.allManagedNodesLocked() {
		report, ok := probed[node.name]
		if !ok {
			continue
		}

		switch {
		case report.Error != nil:
			node.setState(NodeStateDown, report.Error)
		case node.role == RolePrimary:
			node.setState(NodeStateReady, nil)
		case node.state == NodeStateDown && c.options.autoRecoverReplicas:
			node.setState(NodeStateReady, nil)
		}
	}

	failedOver := false
	promotedTo := ""
	if c.primary != nil && c.primary.state == NodeStateDown && c.options.failoverMode == FailoverAutomatic {
		candidate := c.firstReadyReplicaLocked()
		if candidate != nil {
			promotedTo = candidate.name
			c.promoteLocked(candidate, errors.New("primary failover"))
			failedOver = true
		}
	}

	finalReports := c.currentReportsLocked(probed)
	c.mu.Unlock()

	report := buildClusterReport(checkedAt, finalReports, failedOver, promotedTo)
	if report.Status == HealthStatusDown && failedOver == false && c.options.failoverMode == FailoverAutomatic {
		return report, errNoFailoverTarget
	}
	return report, nil
}

func (c *Cluster) DrainReplica(name string, cause error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	replica := c.findReplicaLocked(name)
	if replica == nil {
		return errReplicaNotFound
	}
	replica.setState(NodeStateDraining, cause)
	return nil
}

func (c *Cluster) RecoverReplica(ctx context.Context, name string) error {
	c.mu.RLock()
	replica := c.findReplicaLocked(name)
	c.mu.RUnlock()
	if replica == nil {
		return errReplicaNotFound
	}

	if err := replica.client.PingContext(ctx); err != nil {
		c.mu.Lock()
		replica.setState(NodeStateDown, err)
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	replica.setState(NodeStateReady, nil)
	c.mu.Unlock()
	return nil
}

func (c *Cluster) MarkPrimaryDown(ctx context.Context, cause error) error {
	c.mu.Lock()
	if c.primary == nil {
		c.mu.Unlock()
		return errPrimaryUnavailable
	}
	c.primary.setState(NodeStateDown, cause)
	auto := c.options.failoverMode == FailoverAutomatic
	c.mu.Unlock()

	if !auto {
		return nil
	}

	_, err := c.Failover(ctx)
	return err
}

func (c *Cluster) Failover(ctx context.Context) (Node, error) {
	c.mu.RLock()
	candidate := c.firstReadyReplicaLocked()
	c.mu.RUnlock()
	if candidate == nil {
		return Node{}, errNoFailoverTarget
	}

	return c.promoteReplica(ctx, candidate.name, errors.New("primary failover"))
}

func (c *Cluster) PromoteReplica(ctx context.Context, name string) (Node, error) {
	return c.promoteReplica(ctx, name, errors.New("manual failover"))
}

func (c *Cluster) Metrics() []MetricSample {
	nodes := c.Nodes()
	metrics := make([]MetricSample, 0, len(nodes)*10)
	for _, node := range nodes {
		metrics = append(metrics, node.Metrics()...)
	}
	return metrics
}

func (c *Cluster) Close() error {
	c.mu.RLock()
	nodes := make([]Node, 0, 1+len(c.replicas))
	if c.primary != nil {
		nodes = append(nodes, c.primary.snapshot())
	}
	nodes = append(nodes, snapshots(c.replicas)...)
	c.mu.RUnlock()

	seen := make(map[*Client]struct{}, len(nodes))
	var errs []error
	for _, node := range nodes {
		if _, ok := seen[node.client]; ok {
			continue
		}
		seen[node.client] = struct{}{}
		if err := node.client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (n Node) Name() string {
	return n.name
}

func (n Node) Role() NodeRole {
	return n.role
}

func (n Node) Client() *Client {
	return n.client
}

func (n Node) State() NodeState {
	return n.state
}

func (n Node) Healthy() bool {
	return n.state == NodeStateReady
}

func (n Node) LastError() error {
	return n.lastError
}

func (n Node) UpdatedAt() time.Time {
	return n.updatedAt
}

func (n Node) HealthCheck(ctx context.Context) HealthReport {
	return n.decorateHealthReport(n.client.healthCheck(ctx, n.name, n.role))
}

func (n Node) Metrics() []MetricSample {
	return n.client.metrics(n.name, n.role)
}

func defaultClusterOptions() clusterOptions {
	return clusterOptions{
		failoverMode:          FailoverManual,
		readFallbackToPrimary: true,
		autoRecoverReplicas:   true,
	}
}

func newManagedNode(name string, role NodeRole, client *Client) *managedNode {
	now := time.Now()
	return &managedNode{
		name:      name,
		role:      role,
		client:    client,
		state:     NodeStateReady,
		updatedAt: now,
	}
}

func (n *managedNode) snapshot() Node {
	return Node{
		name:      n.name,
		role:      n.role,
		client:    n.client,
		state:     n.state,
		lastError: n.lastError,
		updatedAt: n.updatedAt,
	}
}

func (n *managedNode) setState(state NodeState, err error) {
	n.state = state
	n.lastError = err
	n.updatedAt = time.Now()
}

func (c *Cluster) allManagedNodesLocked() []*managedNode {
	nodes := make([]*managedNode, 0, 1+len(c.replicas))
	if c.primary != nil {
		nodes = append(nodes, c.primary)
	}
	nodes = append(nodes, c.replicas...)
	return nodes
}

func (c *Cluster) currentReportsLocked(probed map[string]HealthReport) []HealthReport {
	nodes := make([]Node, 0, 1+len(c.replicas))
	if c.primary != nil {
		nodes = append(nodes, c.primary.snapshot())
	}
	nodes = append(nodes, snapshots(c.replicas)...)
	reports := make([]HealthReport, 0, len(nodes))
	for _, node := range nodes {
		report, ok := probed[node.name]
		if !ok {
			report = HealthReport{
				Name:      node.name,
				Role:      node.role,
				State:     node.state,
				Status:    HealthStatusUp,
				CheckedAt: time.Now(),
				Error:     node.lastError,
			}
		}
		reports = append(reports, node.decorateHealthReport(report))
	}
	return reports
}

func (c *Cluster) readyReplicasLocked() []*managedNode {
	ready := make([]*managedNode, 0, len(c.replicas))
	for _, replica := range c.replicas {
		if replica.state == NodeStateReady {
			ready = append(ready, replica)
		}
	}
	return ready
}

func (c *Cluster) firstReadyReplicaLocked() *managedNode {
	for _, replica := range c.replicas {
		if replica.state == NodeStateReady {
			return replica
		}
	}
	return nil
}

func (c *Cluster) findReplicaLocked(name string) *managedNode {
	for _, replica := range c.replicas {
		if replica.name == name {
			return replica
		}
	}
	return nil
}

func (c *Cluster) promoteReplica(ctx context.Context, name string, cause error) (Node, error) {
	c.mu.RLock()
	replica := c.findReplicaLocked(name)
	c.mu.RUnlock()
	if replica == nil {
		return Node{}, errReplicaNotFound
	}

	if err := replica.client.PingContext(ctx); err != nil {
		c.mu.Lock()
		replica.setState(NodeStateDown, err)
		c.mu.Unlock()
		return Node{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	replica = c.findReplicaLocked(name)
	if replica == nil {
		return Node{}, errReplicaNotFound
	}

	return c.promoteLocked(replica, cause), nil
}

func (c *Cluster) promoteLocked(candidate *managedNode, cause error) Node {
	oldPrimary := c.primary
	if oldPrimary != nil {
		oldPrimary.role = RoleReplica
		if oldPrimary.state == NodeStateReady {
			oldPrimary.setState(NodeStateDraining, cause)
		} else if cause != nil {
			oldPrimary.setState(oldPrimary.state, errors.Join(oldPrimary.lastError, cause))
		}
	}

	candidate.role = RolePrimary
	candidate.setState(NodeStateReady, nil)
	c.removeReplicaLocked(candidate.name)
	c.primary = candidate

	if oldPrimary != nil {
		c.replicas = append(c.replicas, oldPrimary)
	}

	return candidate.snapshot()
}

func (c *Cluster) removeReplicaLocked(name string) {
	for i, replica := range c.replicas {
		if replica.name != name {
			continue
		}
		c.replicas = append(c.replicas[:i], c.replicas[i+1:]...)
		return
	}
}

func (n Node) decorateHealthReport(report HealthReport) HealthReport {
	report.Name = n.name
	report.Role = n.role
	report.State = n.state

	switch n.state {
	case NodeStateDraining:
		if report.Status == HealthStatusUp {
			report.Status = HealthStatusDegraded
		}
		if report.Error == nil {
			report.Error = n.lastError
		}
	case NodeStateDown:
		report.Status = HealthStatusDown
		if report.Error == nil {
			report.Error = n.lastError
		}
	}

	return report
}

func probeNodes(ctx context.Context, nodes []Node) []HealthReport {
	probed := make([]HealthReport, len(nodes))
	var wg sync.WaitGroup
	wg.Add(len(nodes))
	for i := range nodes {
		go func(index int) {
			defer wg.Done()
			probed[index] = nodes[index].HealthCheck(ctx)
		}(i)
	}
	wg.Wait()
	return probed
}

func probeNodesByName(ctx context.Context, nodes []Node) map[string]HealthReport {
	reports := probeNodes(ctx, nodes)
	indexed := make(map[string]HealthReport, len(reports))
	for _, report := range reports {
		indexed[report.Name] = report
	}
	return indexed
}

func buildClusterReport(checkedAt time.Time, reports []HealthReport, failedOver bool, promotedTo string) ClusterHealthReport {
	report := ClusterHealthReport{
		Status:     HealthStatusUp,
		CheckedAt:  checkedAt,
		Nodes:      reports,
		FailedOver: failedOver,
		PromotedTo: promotedTo,
	}

	for _, node := range reports {
		if node.Role == RolePrimary && node.Status != HealthStatusUp {
			report.Status = HealthStatusDown
			return report
		}
		if node.Status != HealthStatusUp {
			report.Status = HealthStatusDegraded
		}
	}

	return report
}

func snapshots(nodes []*managedNode) []Node {
	snapshots := make([]Node, len(nodes))
	for i, node := range nodes {
		snapshots[i] = node.snapshot()
	}
	return snapshots
}

func replicaName(index int) string {
	return "replica-" + strconv.Itoa(index+1)
}
