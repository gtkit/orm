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
- **自定义健康探针** — 可注入业务级 probe（`SELECT 1`、只读状态、复制延迟等）
- **读写路由** — Round-robin 副本负载均衡，可回退主库，副本自动恢复
- **读写一致性** — `ContextWithWriteFlag` 标记写后 context，后续读自动走主库，零额外开销
- **显式拓扑切换** — 外部 HA 系统完成切换后，`SwitchPrimary()` 更新路由视图
- **Replica 并行打开** — 集群初始化时并行连接所有副本，减少启动耗时
- **启动期 Ping Retry** — 可选 backoff 重试，降低容器冷启动对数据库瞬时波动的敏感度

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

启动期如果希望容忍数据库短暂不可用，可开启 ping retry：

```go
client, err := orm.Open(
    context.Background(),
    orm.WithHost("127.0.0.1"),
    orm.WithDatabase("orders"),
    orm.WithUser("root"),
    orm.WithPassword("secret"),
    orm.WithStartupPingRetry(5, 200*time.Millisecond, 2*time.Second),
)
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
    orm.WithRetryMaxWait(100*time.Millisecond),
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

如果你需要观测死锁重试次数、退避时长和触发点，可注入 observer：

```go
client, err := orm.Open(
    ctx,
    orm.WithTxRetryObserver(func(ctx context.Context, event orm.TxRetryEvent) {
        metrics.ObserveDeadlockRetry(event.ClientName, event.Attempt, event.Wait)
    }),
)
```

`WithTx()` 回调内部如果发生 panic，库会先执行 `Rollback()`，再继续 `panic` 抛给调用方；调用方仍应在上层按自己的标准恢复或记录 panic。

如果你希望限制单次退避上限，可使用：

```go
err := client.WithTx(ctx, nil, fn,
    orm.WithMaxRetries(5),
    orm.WithRetryBaseWait(10*time.Millisecond),
    orm.WithRetryMaxWait(100*time.Millisecond),
)
```

## 读写一致性保护

写主库后立刻从副本读可能拿到旧数据（副本有复制延迟）。通过 `ContextWithWriteFlag` 标记写后的 context，后续读请求自动路由到主库：

```go
// 1. 写操作
err := cluster.WithTx(ctx, func(tx *gorm.DB) error {
    return tx.Create(&order).Error
})

// 2. 标记写后 context
ctx = orm.ContextWithWriteFlag(ctx)

// 3. 后续读走主库，保证读到最新数据
client, err := cluster.ReaderClientCtx(ctx)
client.DB().WithContext(ctx).First(&order, orderID)
```

如果当前请求上下文比较长，应使用窗口期或显式清除：

```go
ctx = orm.ContextWithWriteWindow(ctx, 2*time.Second)
// 或者
ctx = orm.ContextClearWriteFlag(ctx)
```

**性能影响：零。** 无 write flag 时 `ReaderClientCtx` 与 `ReaderClient` 耗时完全一致（~27ns），有 write flag 时反而更快（~15ns，跳过副本遍历）。

注意：不要在 websocket、长轮询、消息消费等长生命周期 context 上长期保留 write flag，否则读流量会不知不觉全部压到主库。

| 方法 | 适用场景 |
|------|---------|
| `ReaderClient()` | 普通读，无需 context |
| `ReaderClientCtx(ctx)` | 需要读写一致性保护的读 |
| `HasWriteFlag(ctx)` | 检查当前 context 是否有写标记 |

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

业务进程可以显式启动后台健康循环：

```go
go func() {
    if err := cluster.RunHealthLoop(ctx, 5*time.Second); err != nil {
        logger.Error("cluster health loop exited", zap.Error(err))
    }
}()
```

优雅停机时，不要先 `cluster.Close()` 再等健康循环自己结束。推荐顺序是：

1. 先 cancel 掉传给 `RunHealthLoop()` 的 `ctx`
2. 等健康循环 goroutine 退出
3. 再调用 `cluster.Close()`

`RunHealthLoop()` 不会自行托管后台 goroutine 生命周期；它是显式阻塞循环，调用方应自己持有 `ctx` 和 goroutine 的退出同步。

如果默认 `Ping()` 不够，可以叠加自定义探针：

```go
client, err := orm.Open(
    ctx,
    orm.WithHealthProbe(func(ctx context.Context, client *orm.Client, role orm.NodeRole) error {
        if role == orm.RoleReplica {
            return client.DB().WithContext(ctx).Exec("SELECT 1").Error
        }
        return nil
    }),
)
```

自定义 probe 适合做以下检查：

- `SELECT 1`
- 副本只读状态
- 平台侧暴露的复制延迟 SQL / 视图
- 业务约束要求的 readiness 校验

指标列表：`orm_db_max_open_connections`, `orm_db_open_connections`, `orm_db_in_use_connections`, `orm_db_idle_connections`, `orm_db_wait_count_total`, `orm_db_wait_duration_seconds_total`, `orm_db_max_idle_closed_total`, `orm_db_max_idle_time_closed_total`, `orm_db_max_lifetime_closed_total`, `orm_db_connection_utilization`

Prometheus / OpenTelemetry 参考实现：

- [Prometheus 接入示例](../docs/prometheus-%E6%8E%A5%E5%85%A5%E7%A4%BA%E4%BE%8B.md)
- [OpenTelemetry 接入示例](../docs/OpenTelemetry-%E6%8E%A5%E5%85%A5%E7%A4%BA%E4%BE%8B.md)

## 集成测试

`v2` 提供了真实 MySQL 集成测试，默认不会在普通 `go test ./...` 中连接数据库；只有显式设置环境变量时才会执行。

本地运行示例：

```bash
cd v2
ORM_TEST_DSN='root:root@tcp(127.0.0.1:3306)/' make integration
```

Lint 需使用 `golangci-lint` v2；仓库根目录的 `.golangci-lint-version` 已固定当前 CI 版本。

环境变量说明：

- `ORM_RUN_INTEGRATION=1`：开启真实数据库集成测试
- `ORM_TEST_DSN`：指向 MySQL 实例的基础 DSN，建议不要固定 schema；测试会自动创建并清理临时数据库

当前集成测试覆盖：

- `Config.Open()` + GORM 实际建表/读写
- `Cluster.WithReadTx()` 在 write flag 下回主库
- `SwitchPrimary()` 后的真实写路由切换

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

`MarkPrimaryDown()` 是一个临时运维状态：如果后续调用 `Refresh()` 且主库 Ping 恢复成功，状态会自动回到 `Ready`。  
如果你的目标是长期隔离主库，请同时停止健康循环，或避免继续触发 `Refresh()`。

**重要警告：** `SwitchPrimary()` 只更新 `Cluster` 内部路由视图，不会让你之前已经拿到的 `*Client` / `*gorm.DB` 自动失效。  
切主完成后，必须重新调用 `Primary()`、`WriteClient()`、`ReaderClient()` 或 `ReaderClientCtx()` 重新取句柄；不要继续复用切主前缓存的写连接。

## 外部连接池包装

```go
client, err := orm.OpenWithDB(ctx, existingSQLDB,
    orm.WithName("legacy"),
    orm.WithStartupPing(false),
)
// Close() 不会关闭外部传入的 *sql.DB
```

## 与 sqlc / go-jet 配合使用

本包只管连接，查询层可自由选择。通过 `SQLDB()` 获取底层 `*sql.DB`，即可接入任何查询工具：

### sqlc

```go
// sqlc 从 SQL 文件生成类型安全的 Go 代码，性能等同原生 database/sql。
// 安装: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

// 单节点
client, _ := orm.Open(ctx, orm.WithHost("127.0.0.1"), ...)
queries := db.New(client.SQLDB())

order, err := queries.GetOrder(ctx, orderID)
```

### go-jet

```go
// go-jet 从数据库 schema 生成类型安全的 Go DSL，编译时检查字段名和类型。
// 安装: go install github.com/go-jet/jet/v2/cmd/jet@latest

client, _ := orm.Open(ctx, orm.WithHost("127.0.0.1"), ...)

stmt := SELECT(Orders.AllColumns).
    FROM(Orders).
    WHERE(Orders.ID.EQ(Int64(orderID)))

var order model.Orders
err := stmt.Query(client.SQLDB(), &order)
```

### 集群模式 — 读写分离

```go
cluster, _ := orm.OpenCluster(ctx, primaryCfg, replicaCfg)

// 写走主库
writeDB := cluster.Primary().SQLDB()
writeQueries := db.New(writeDB)
writeQueries.CreateOrder(ctx, params)

// 读走副本
readDB := cluster.Reader().SQLDB()
readQueries := db.New(readDB)
orders, _ := readQueries.ListOrders(ctx)

// 写后读一致性 — 走主库
ctx = orm.ContextWithWriteFlag(ctx)
reader, _ := cluster.ReaderClientCtx(ctx)
consistentQueries := db.New(reader.SQLDB())
order, _ := consistentQueries.GetOrder(ctx, orderID)
```

### 混合使用 GORM + sqlc

```go
client, _ := orm.Open(ctx, ...)

// 简单 CRUD 用 GORM
client.DB().Create(&user)

// 热路径复杂查询用 sqlc（零反射开销）
queries := db.New(client.SQLDB())
stats, _ := queries.GetDashboardStats(ctx)
```

### 性能选型参考

| 方案 | 性能 | 适合场景 |
|------|------|---------|
| GORM | 基准 | 标准 CRUD、快速开发 |
| GORM `db.Raw()` | ≈原生 | GORM 项目中的少量热路径 |
| sqlc | ≈原生 | SQL 先行、高 QPS 热路径 |
| go-jet | 接近原生 | 复杂动态查询、类型安全 |

## 设计原则

- 主从拓扑切换交给数据库平台或外部 HA 系统
- 应用侧只负责连接管理、健康检查、读写路由和显式拓扑同步
- `SwitchPrimary()` 只更新应用内路由视图，不修改数据库真实角色

## 使用注意

- `PrepareStmt=true` 配合读写分离时，要求主库和副本在 `sql_mode`、字符集、时区等会影响 prepare 行为的配置上保持一致；否则建议关闭 `PrepareStmt`
- 本库输出的是通用 `MetricSample` 和 retry observer，不直接绑定 Prometheus / OpenTelemetry；如果你的监控体系要求 `prometheus.Collector` 或 span，请在接入层做适配
- `Close()` 是立即关闭连接池，不等待外部已经拿到的查询自动排空；优雅停机请在应用层先停止流量，再关闭 `Cluster` / `Client`
