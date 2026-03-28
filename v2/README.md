# orm/v2

`v2` 是这个仓库唯一面向生产使用的实现。

它负责:

- 单节点 MySQL/GORM 连接管理
- 连接池配置
- 事务包装
- 健康检查和连接池指标
- 已知主从拓扑下的读写路由
- 运维显式触发的主节点切换

它不负责:

- 自动故障转移
- 自动升主
- 真正的数据库拓扑编排

如果你需要数据库级别的主从切换，请交给外部 HA 编排系统；应用侧只在拓扑已经切换完成后调用 `SwitchPrimary()` 更新路由视图。

## 依赖要求

- Go `1.26`

## 单节点示例

```go
package main

import (
	"context"
	"time"

	orm "github.com/gtkit/orm/v2"
	ormzap "github.com/gtkit/orm/v2/zlogger"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	client, err := orm.Open(
		context.Background(),
		orm.WithName("orders-primary"),
		orm.WithHost("127.0.0.1"),
		orm.WithPort("3306"),
		orm.WithDatabase("orders"),
		orm.WithUser("root"),
		orm.WithPassword("secret"),
		orm.WithMaxOpenConns(50),
		orm.WithMaxIdleConns(10),
		orm.WithConnMaxLifetime(30*time.Minute),
		orm.WithConnMaxIdleTime(10*time.Minute),
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

	_ = client.DB()
}
```

## 外部连接池包装

如果你的项目已经自行管理 `*sql.DB`，可以直接包装:

```go
client, err := orm.OpenWithDB(
	context.Background(),
	sqlDB,
	orm.WithName("legacy-primary"),
	orm.WithStartupPing(false),
	orm.WithSkipInitializeWithVersion(true),
)
```

`OpenWithDB()` 不会接管外部 `*sql.DB` 的关闭动作。

## Cluster 模式

### 创建

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
```

### 默认行为

- 写流量永远走当前 primary
- 读流量优先走健康 replica
- 没有健康 replica 时，可回退到 primary
- `Refresh()` 只刷新节点状态，不会自动切主

### 显式配置

```go
cluster, err := orm.OpenClusterWithOptions(
	context.Background(),
	primary,
	[]orm.Config{replicaA},
	orm.WithReadFallbackToPrimary(true),
	orm.WithAutoRecoverReplicas(true),
)
```

### 读写路由

```go
writeDB := cluster.WriteDB()
readDB := cluster.ReadDB()
_, _ = writeDB, readDB
```

### 健康刷新

```go
report := cluster.Refresh(context.Background())
if report.Status == orm.HealthStatusDown {
	// 当前主节点不可写
}
```

### 副本摘除与恢复

```go
if err := cluster.DrainReplica("replica-a", errors.New("replication lag")); err != nil {
	panic(err)
}

if err := cluster.RecoverReplica(context.Background(), "replica-a"); err != nil {
	panic(err)
}
```

### 主节点不可用

如果业务或运维已经确认当前主节点不可写，可以显式标记:

```go
if err := cluster.MarkPrimaryDown(errors.New("write timeout")); err != nil {
	panic(err)
}
```

这只会停止应用继续把写流量发往当前 primary，不会自动切主。

### 外部编排后的主节点切换

当外部系统已经完成数据库拓扑切换后，应用侧显式更新路由:

```go
node, err := cluster.SwitchPrimary(context.Background(), "replica-a")
if err != nil {
	panic(err)
}

_ = node
```

`SwitchPrimary()` 只更新应用内的读写路由视图。它不会修改数据库的真实角色。

## logger

只保留 `zap` 适配:

- `WithLogger`
- `WithLogLevel`
- `WithSlowThreshold`
- `WithIgnoreRecordNotFoundError`
- `WithParameterizedQueries`

`LogMode()` 语义与 GORM 保持一致，`db.Debug()` 也会生效。

## 生产建议

- 主从拓扑切换交给数据库平台或外部编排系统
- 应用侧只负责连接管理、健康检查、读写路由和显式拓扑同步
- 需要更重的读写分离能力时，可以评估 GORM 官方 `DBResolver`
