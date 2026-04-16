# orm/v2

生产级 Go GORM MySQL 连接管理，支持单节点和集群模式。

## 安装

```bash
go get github.com/gtkit/orm/v2
```

## 特性

- **单节点 & 集群** — 统一的 Client 抽象，按需扩展到读写分离
- **安全默认值** — 连接池（MaxOpen=50, MaxIdle=10）、读写超时（30s）、健康检查超时（5s）
- **密码防泄露** — JSON/YAML 序列化自动隐藏，`String()`/`GoString()` 自动脱敏
- **死锁自动重试** — `WithTx` 检测 MySQL 1213/1205 自动重试，指数退避 + 随机抖动
- **并发安全** — epoch 机制防止 TOCTOU 竞态，所有共享状态锁内访问
- **Trace ID 链路追踪** — `WithTraceIDExtractor` 将请求 ID 注入每条 SQL 日志
- **健康检查 & 指标** — Ping 探活 + 10 项连接池指标，超时可配置
- **读写路由** — Round-robin 副本负载均衡，可回退主库，副本自动恢复
- **显式拓扑切换** — 外部 HA 系统完成切换后，`SwitchPrimary()` 更新路由视图
- **Replica 并行打开** — 集群初始化时并行连接所有副本，减少启动耗时

## 快速开始

### 单节点

```go
client, err := orm.Open(
    context.Background(),
    orm.WithHost("127.0.0.1"),
    orm.WithPort("3306"),
    orm.WithDatabase("orders"),
    orm.WithUser("root"),
    orm.WithPassword("secret"),
    orm.WithPrepareStmt(true),
    orm.WithSkipDefaultTransaction(true),
    orm.WithGormLogger(
        ormzap.New(
            ormzap.WithLogLevel(gormlogger.Warn),
            ormzap.WithSlowThreshold(200*time.Millisecond),
            ormzap.WithIgnoreRecordNotFoundError(true),
            ormzap.WithParameterizedQueries(true),
        ),
    ),
)
if err != nil {
    panic(err)
}
defer client.Close()

db := client.DB() // *gorm.DB
```

### 集群模式

```go
primary := orm.NewConfig(
    orm.WithName("primary"),
    orm.WithHost("10.0.0.10"),
    orm.WithDatabase("app"),
    orm.WithUser("root"),
    orm.WithPassword("secret"),
)

replicaA := orm.NewConfig(
    orm.WithName("replica-a"),
    orm.WithHost("10.0.0.11"),
    orm.WithDatabase("app"),
    orm.WithUser("root"),
    orm.WithPassword("secret"),
)

cluster, err := orm.OpenCluster(context.Background(), primary, replicaA)
if err != nil {
    panic(err)
}
defer cluster.Close()

writeDB := cluster.WriteDB()
readDB := cluster.ReadDB()
```

### 集群选项

```go
cluster, err := orm.OpenClusterWithOptions(
    context.Background(),
    primary,
    []orm.Config{replicaA, replicaB},
    orm.WithReadFallbackToPrimary(true),    // 无健康副本时回退主库（默认: true）
    orm.WithAutoRecoverReplicas(true),      // Refresh 时自动恢复副本（默认: true）
    orm.WithHealthCheckTimeout(5*time.Second), // 健康检查超时（默认: 5s）
)
```

## 事务 & 死锁重试

```go
// 默认：死锁自动重试 3 次，指数退避 5ms-50ms
err := client.WithTx(ctx, nil, func(tx *gorm.DB) error {
    return tx.Create(&order).Error
})

// 自定义重试策略
err := client.WithTx(ctx, nil, fn,
    orm.WithMaxRetries(5),
    orm.WithRetryBaseWait(10*time.Millisecond),
)

// 禁用重试
err := client.WithTx(ctx, nil, fn, orm.WithMaxRetries(0))

// 只读事务
err := client.WithReadTx(ctx, func(tx *gorm.DB) error {
    return tx.Find(&users).Error
})

// 集群事务 — 写事务走主库，读事务走副本
err := cluster.WithTx(ctx, func(tx *gorm.DB) error { ... })
err := cluster.WithReadTx(ctx, func(tx *gorm.DB) error { ... })
```

## Trace ID 链路追踪

```go
ormzap.New(
    ormzap.WithLogger(myZapLogger),
    ormzap.WithTraceIDExtractor(func(ctx context.Context) string {
        if id, ok := ctx.Value("X-Request-ID").(string); ok {
            return id
        }
        return ""
    }),
)
```

所有 SQL 日志将自动携带 `trace_id` 字段，可关联请求链路。

## 健康检查 & 指标

```go
// 单节点
report := client.HealthCheck(ctx)
metrics := client.Metrics()

// 集群
report := cluster.HealthCheck(ctx) // 并行探测所有节点
report := cluster.Refresh(ctx)     // 探测 + 更新节点状态
metrics := cluster.Metrics()       // 所有节点指标
```

指标列表：`orm_db_max_open_connections`, `orm_db_open_connections`, `orm_db_in_use_connections`, `orm_db_idle_connections`, `orm_db_wait_count_total`, `orm_db_wait_duration_seconds_total`, `orm_db_max_idle_closed_total`, `orm_db_max_idle_time_closed_total`, `orm_db_max_lifetime_closed_total`, `orm_db_connection_utilization`

## 运维操作

```go
// 摘除副本
cluster.DrainReplica("replica-a", errors.New("replication lag"))

// 恢复副本
cluster.RecoverReplica(ctx, "replica-a")

// 标记主库不可用（不会自动切主）
cluster.MarkPrimaryDown(errors.New("write timeout"))

// 外部 HA 完成切换后，更新应用路由
cluster.SwitchPrimary(ctx, "replica-a")
```

## 外部连接池包装

```go
client, err := orm.OpenWithDB(ctx, existingSQLDB,
    orm.WithName("legacy"),
    orm.WithStartupPing(false),
)
// Close() 不会关闭外部传入的 *sql.DB
```

## 设计原则

- 主从拓扑切换交给数据库平台或外部 HA 系统
- 应用侧只负责连接管理、健康检查、读写路由和显式拓扑同步
- `SwitchPrimary()` 只更新应用内路由视图，不修改数据库真实角色
