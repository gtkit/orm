# orm/v2

`v2` 是独立维护的实例化版本，对外路径为 `github.com/gtkit/orm/v2`。

它适合生产项目直接使用，设计目标是：

- 不使用包级全局配置，单进程可安全维护多个数据库实例
- 支持直接打开新连接，或包装已有 `*sql.DB`
- 支持事务包装、健康检查、连接池指标导出
- 支持主从集群、读写分离、故障切换、副本摘除与恢复

## 依赖要求

- Go `1.26`
- 直接依赖版本见 `go.mod`

## 核心对象

### `Config`

单个数据库实例的配置对象，包含：

- MySQL 连接参数
- 连接池参数
- GORM 参数
- MySQL Dialect 参数
- 启动探活参数

### `Client`

单个数据库实例运行对象，提供：

- `DB()` 获取 `*gorm.DB`
- `SQLDB()` 获取 `*sql.DB`
- `WithTx()` / `WithReadTx()` 事务包装
- `HealthCheck()` 健康检查
- `Metrics()` 连接池指标样本
- `Close()` 关闭自己创建的连接池

### `Cluster`

主从集群对象，提供：

- `WriteDB()` 写库连接
- `ReadDB()` 读库连接
- `ReaderClient()` 健康感知的读实例选择
- `Refresh()` 按策略刷新节点状态并执行自动故障切换
- `DrainReplica()` / `RecoverReplica()` 副本摘除与恢复
- `PromoteReplica()` / `Failover()` 手动主备切换

## 单实例使用

### 1. 直接打开连接

```go
package main

import (
	"context"
	"time"

	orm "github.com/gtkit/orm/v2"
)

func main() {
	cfg := orm.NewConfig(
		orm.WithName("orders-primary"),
		orm.WithHost("127.0.0.1"),
		orm.WithPort("3306"),
		orm.WithDatabase("orders"),
		orm.WithUser("root"),
		orm.WithPassword("root123"),
		orm.WithMaxOpenConns(50),
		orm.WithMaxIdleConns(10),
		orm.WithConnMaxLifetime(30*time.Minute),
		orm.WithConnMaxIdleTime(10*time.Minute),
		orm.WithPrepareStmt(true),
		orm.WithSkipDefaultTransaction(true),
		orm.WithTablePrefix("t_"),
		orm.WithSingularTable(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := cfg.Open(ctx)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	db := client.DB()
	_ = db
}
```

### 2. 包装已有连接池

如果你的项目已经自己管理了 `*sql.DB`，推荐直接包装：

```go
package main

import (
	"context"
	"database/sql"

	orm "github.com/gtkit/orm/v2"
)

func useExisting(sqlDB *sql.DB) error {
	client, err := orm.OpenWithDB(
		context.Background(),
		sqlDB,
		orm.WithName("legacy-primary"),
		orm.WithStartupPing(false),
		orm.WithSkipInitializeWithVersion(true),
	)
	if err != nil {
		return err
	}

	db := client.DB()
	_ = db
	return nil
}
```

注意：

- `Open()` 创建的连接池由 `Client.Close()` 负责关闭
- `OpenWithDB()` 包装外部连接池时，`Client.Close()` 不会关闭外部 `*sql.DB`

## 启动探活与懒连接

默认行为：

- `StartupPing = true`
- `gorm.Config.DisableAutomaticPing = true`

这意味着：

- 初始化是否探活由本库控制，而不是交给 GORM 隐式完成
- 默认会在启动时主动 `PingContext`

如果你希望初始化阶段完全不触发数据库探活，需要同时设置：

- `orm.WithStartupPing(false)`
- `orm.WithSkipInitializeWithVersion(true)`

示例：

```go
client, err := orm.Open(
	ctx,
	orm.WithHost("127.0.0.1"),
	orm.WithPort("3306"),
	orm.WithDatabase("app"),
	orm.WithUser("root"),
	orm.WithPassword("secret"),
	orm.WithStartupPing(false),
	orm.WithSkipInitializeWithVersion(true),
)
```

## 常用配置项

连接参数：

- `WithName`
- `WithHost`
- `WithPort`
- `WithAddress`
- `WithDatabase`
- `WithUser`
- `WithPassword`
- `WithTimeout`
- `WithReadTimeout`
- `WithWriteTimeout`
- `WithTLSConfig`
- `WithDSNParam`
- `WithDSNParams`

连接池参数：

- `WithMaxOpenConns`
- `WithMaxIdleConns`
- `WithConnMaxLifetime`
- `WithConnMaxIdleTime`

GORM 参数：

- `WithPrepareStmt`
- `WithPrepareStmtCache`
- `WithSkipDefaultTransaction`
- `WithGormLogger`
- `WithNowFunc`
- `WithNamingStrategy`
- `WithTablePrefix`
- `WithSingularTable`
- `WithDefaultContextTimeout`
- `WithDefaultTransactionTimeout`
- `WithDryRun`
- `WithQueryFields`
- `WithCreateBatchSize`
- `WithTranslateError`

## 事务包装

写事务：

```go
err := client.WithTx(ctx, nil, func(tx *gorm.DB) error {
	if err := tx.Create(&user).Error; err != nil {
		return err
	}
	return tx.Model(&order).Update("status", "paid").Error
})
```

只读事务：

```go
err := client.WithReadTx(ctx, func(tx *gorm.DB) error {
	return tx.First(&user, 1).Error
})
```

行为说明：

- `fn` 返回错误时自动回滚
- `fn` panic 时自动回滚后继续 panic
- `Commit` 失败会直接返回提交错误

## 健康检查与指标

### 单实例健康检查

```go
report := client.HealthCheck(ctx)
if !report.Healthy() {
	panic(report.Error)
}
```

返回内容包括：

- 实例名 `Name`
- 角色 `Role`
- 状态 `State`
- 健康结果 `Status`
- 检查耗时 `Duration`
- 连接池快照 `Stats`

### 连接池指标导出

```go
samples := client.Metrics()
for _, sample := range samples {
	// sample.Name
	// sample.Value
	// sample.Labels
}
```

当前会导出这些指标名：

- `orm_db_max_open_connections`
- `orm_db_open_connections`
- `orm_db_in_use_connections`
- `orm_db_idle_connections`
- `orm_db_wait_count_total`
- `orm_db_wait_duration_seconds_total`
- `orm_db_max_idle_closed_total`
- `orm_db_max_idle_time_closed_total`
- `orm_db_max_lifetime_closed_total`
- `orm_db_connection_utilization`

## 集群模式

### 1. 基础主从集群

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

replicaB := orm.NewConfig(
	orm.WithName("replica-b"),
	orm.WithHost("10.0.0.12"),
	orm.WithDatabase("app"),
	orm.WithUser("root"),
	orm.WithPassword("secret"),
)

cluster, err := orm.OpenCluster(ctx, primary, replicaA, replicaB)
if err != nil {
	panic(err)
}
defer cluster.Close()
```

默认行为：

- 写流量走当前主库
- 读流量优先走健康副本
- 没有可读副本时，读流量回退到主库
- 主库故障不会自动切换，默认是手动模式

### 2. 显式配置集群策略

```go
cluster, err := orm.OpenClusterWithOptions(
	ctx,
	primary,
	[]orm.Config{replicaA, replicaB},
	orm.WithFailoverMode(orm.FailoverAutomatic),
	orm.WithReadFallbackToPrimary(true),
	orm.WithAutoRecoverReplicas(true),
)
```

策略说明：

- `FailoverManual`
  不自动切主，适合希望由业务或运维显式控制的场景
- `FailoverAutomatic`
  在 `Refresh()` 发现主库不可用时，自动提升一个健康副本为新主库
- `WithReadFallbackToPrimary(false)`
  没有可用副本时，不允许读回退到主库
- `WithAutoRecoverReplicas(true)`
  `Refresh()` 发现副本恢复正常后，会自动把 `down` 副本恢复为 `ready`

### 3. 读写分离

```go
writeDB := cluster.WriteDB()
readDB := cluster.ReadDB()
_, _ = writeDB, readDB
```

如果你需要显式判断当前是否有可读节点：

```go
reader, err := cluster.ReaderClient()
if err != nil {
	// 没有可读节点
}
_ = reader
```

### 4. 集群事务

```go
err = cluster.WithTx(ctx, func(tx *gorm.DB) error {
	return tx.Create(&user).Error
})

err = cluster.WithReadTx(ctx, func(tx *gorm.DB) error {
	return tx.First(&user, 1).Error
})
```

行为说明：

- `WithTx()` 永远使用当前主库
- `WithReadTx()` 使用当前读节点选择结果

## 故障切换策略

### 1. 手动故障切换

手动将指定副本提升为主库：

```go
promoted, err := cluster.PromoteReplica(ctx, "replica-a")
if err != nil {
	panic(err)
}

_ = promoted
```

手动触发自动选择一个健康副本切主：

```go
promoted, err := cluster.Failover(ctx)
if err != nil {
	panic(err)
}

_ = promoted
```

切换后的行为：

- 被提升的副本会成为新的 `primary`
- 原主库会降级为 `replica`
- 原主库如果之前是健康的，会进入 `draining`
- 原主库如果已经故障，会保持 `down`

### 2. 自动故障切换

自动故障切换不会在后台偷偷执行，它是在你调用 `Refresh()` 时按策略执行。

示例：

```go
report, err := cluster.Refresh(ctx)
if err != nil {
	// 例如主库挂了，但没有健康副本可接管
}

if report.FailedOver {
	// report.PromotedTo 是新主库名称
}
```

`Refresh()` 会做这些事：

- 主动探测所有节点健康状态
- 将探测失败的节点标记为 `down`
- 按策略自动恢复可恢复副本
- 如果主库 `down` 且启用了 `FailoverAutomatic`，尝试提升一个健康副本为新主库

### 3. 写请求失败后手动标记主库下线

如果你在业务写路径中已经确认主库不可用，可以直接标记并触发自动切换：

```go
err := cluster.MarkPrimaryDown(ctx, errors.New("write timeout"))
if err != nil {
	panic(err)
}
```

如果当前策略是 `FailoverAutomatic`，它会立即尝试切主。

## 副本摘除与恢复

### 1. 摘除副本

当副本延迟过高、数据不一致或你准备维护时，可以先摘除：

```go
err := cluster.DrainReplica("replica-a", errors.New("replication lag too high"))
if err != nil {
	panic(err)
}
```

摘除后的行为：

- 节点状态变为 `draining`
- 该副本不会再参与读路由
- 读请求会自动切到其他健康副本，或按策略回退到主库

### 2. 恢复副本

```go
err := cluster.RecoverReplica(ctx, "replica-a")
if err != nil {
	panic(err)
}
```

恢复逻辑：

- 先主动 `Ping`
- 成功则状态恢复为 `ready`
- 失败则状态更新为 `down`

### 3. 节点状态说明

- `ready`
  节点健康，可参与路由
- `draining`
  节点被人工摘除，不参与路由，但连接仍保留
- `down`
  节点探测失败或已被判定不可用

## 集群健康检查

只观测，不修改集群拓扑：

```go
report := cluster.HealthCheck(ctx)
if report.Status == orm.HealthStatusDown {
	panic("primary unavailable")
}
```

如果要在健康巡检中同时自动更新状态并触发故障切换，使用：

```go
report, err := cluster.Refresh(ctx)
if err != nil {
	// 例如没有可切换副本
}
_ = report
```

## 生产建议

- 主库故障切换如果涉及跨机房或一致性要求高的业务，建议优先使用 `FailoverManual`
- 如果启用 `FailoverAutomatic`，建议配合外部监控或心跳任务定时调用 `Refresh()`
- 对延迟敏感的读业务，建议在副本出现延迟时先 `DrainReplica()`，恢复后再 `RecoverReplica()`
- 如果你已经有独立的连接池生命周期管理，优先使用 `OpenWithDB()`
