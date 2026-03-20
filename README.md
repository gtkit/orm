# orm

### gorm链接数据库

```go
var mdb *gorm.DB
orm.MysqlConfig(
	orm.Host("127.0.0.1"),               // 数据库地址
	orm.Port("3306"),                    // 数据库端口
	orm.DbType("mysql"),                 // 数据库类型
	orm.Name("office_aid"),              // 数据库名称
	orm.User("root"),                    // 数据库用户名
	orm.WithPassword("root123"),         // 数据库密码
	orm.MaxOpenConns(50),                // 最大打开连接数
	orm.MaxIdleConns(10),                // 最大空闲连接数
	orm.ConnMaxLifetime(30*time.Minute), // 连接最大存活时间
	orm.ConnMaxIdleTime(10*time.Minute), // 连接最大空闲时间
)
orm.GormConfig(
	orm.PrepareStmt(true),                 // 是否预编译 SQL 语句
	orm.SkipDefaultTransaction(true),      // 是否跳过默认事务
	orm.GormLog(gormlog.New(logger.Zlog())), // 此处注意,日志需要先实例化
	orm.NowFunc(f func() time.Time),       // 此处注意,自定义 now 时间函数
	orm.SingularTable(true),               // 表名不加复数
	orm.TablePrefix("t_"),                 // 表名前缀
)

// 推荐: 返回 error 由业务自行处理
mdb, err = orm.OpenMysql()
if err != nil {
	panic(err)
}

// 兼容旧用法: 出错直接 panic
// mdb = orm.NewMysql()

// 可继续使用 setter 做额外自定义
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
