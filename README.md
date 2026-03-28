# orm

根模块保留旧版兼容 API。

生产项目请直接使用 [`github.com/gtkit/orm/v2`](./v2)。

## 当前状态

- 根模块: 兼容层，不再继续扩展架构能力
- `v2`: 生产主线
- 内置日志适配: 只保留 `zap`
- 不再提供 `slog` 适配

## 旧版用法

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

## 迁移建议

- 新项目不要再基于根模块扩展
- 多实例、多数据库、连接池生命周期管理请切换到 `v2`
- 如果你需要线上可维护的读写路由、健康检查和显式拓扑切换，只用 `v2`
