# orm

### gorm链接数据库

```go
var mdb *gorm.DB
orm.MysqlConfig(
		orm.Host("127.0.0.1"),
		orm.Port("3306"),
		orm.DbType("mysql"),
		orm.Name("office_aid"),
		orm.User("root"),
	)
orm.GormConfig(
		orm.PrepareStmt(true),
		orm.SkipDefaultTransaction(true),
		orm.GormLog(gormlog.New(logger.Zlog())), // 此处注意,日志需要先实例化
                orm.NowFunc(f func() time.Time)
	)

mdb = orm.NewMysql() // 实例化数据库连接, 并返回*gorm.DB, 可配置实现 setter 接口
// mdb = orm.NewMysql(&DBSet{})
```

```go
type DBSet struct{}

func (d *DBSet) Set(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}

	// 设置最大打开连接数
	sqlDB.SetMaxOpenConns(config.GetInt("database.maxopenconn"))
	// 用于设置闲置的连接数.设置闲置的连接数则当开启的一个连接使用完成后可以放在池里等候下一次使用
	sqlDB.SetMaxIdleConns(config.GetInt("database.maxidleconn"))
	// 设置每个链接的过期时间
	sqlDB.SetConnMaxLifetime(time.Duration(config.GetInt("database.maxlifeseconds")) * time.Second)
	logger.Info("Mysql Custom set done!")
}
```
# gorm自定义数据类型

sqlite, mysql, postgres supported

```
https://github.com/go-gorm/datatypes
```
