# orm

Go GORM MySQL 连接管理工具。提供开箱即用的连接配置、连接池管理和日志集成。

如需集群读写路由、健康检查、主节点切换，请使用 [`github.com/gtkit/orm/v2`](./v2)。

## 安装

```bash
go get github.com/gtkit/orm
```

## 特性

- 全局配置，一行初始化
- 安全的连接池默认值（MaxOpen=50, MaxIdle=10, Lifetime=30m, IdleTime=10m）
- 读写超时默认值（ReadTimeout=30s, WriteTimeout=30s）
- 密码防泄露（JSON/YAML 序列化自动隐藏，`String()` 自动脱敏）
- Zap 日志适配，支持慢查询检测和 Trace ID 链路追踪
- `RedactedDSN()` 安全日志输出

## 快速开始

```go
package main

import (
	"time"

	"github.com/gtkit/orm"
	gormlogger "gorm.io/gorm/logger"

	ormzap "github.com/gtkit/orm/zlogger"
)

func main() {
	orm.MysqlConfig(
		orm.Host("127.0.0.1"),
		orm.Port("3306"),
		orm.Name("app"),
		orm.User("root"),
		orm.WithPassword("secret"),
		orm.MaxOpenConns(50),
		orm.MaxIdleConns(10),
		orm.ConnMaxLifetime(30*time.Minute),
		orm.ConnMaxIdleTime(10*time.Minute),
	)

	orm.GormConfig(
		orm.PrepareStmt(true),
		orm.SkipDefaultTransaction(true),
		orm.GormLog(
			ormzap.New(
				ormzap.WithLogLevel(gormlogger.Warn),
				ormzap.WithSlowThreshold(200*time.Millisecond),
				ormzap.WithIgnoreRecordNotFoundError(true),
			),
		),
		orm.SingularTable(true),
		orm.TablePrefix("t_"),
	)

	db, err := orm.OpenMysql()
	if err != nil {
		panic(err)
	}

	_ = db
}
```

## 带 Close 的用法

```go
result, err := orm.OpenMysqlWithClose()
if err != nil {
    panic(err)
}
defer result.Close()

db := result.DB
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

所有 SQL 日志将自动携带 `trace_id` 字段。

## v1 与 v2 对比

| 能力 | v1 | v2 |
|------|----|----|
| 单节点连接 | ✅ | ✅ |
| 连接池管理 | ✅ | ✅ |
| Zap 日志 | ✅ | ✅ |
| Trace ID | ✅ | ✅ |
| 密码防泄露 | ✅ | ✅ |
| 死锁自动重试 | - | ✅ |
| 集群读写路由 | - | ✅ |
| 健康检查 | - | ✅ |
| 主节点切换 | - | ✅ |
| 多实例 | - | ✅ |
