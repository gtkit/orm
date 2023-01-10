# orm

gorm链接数据库

```
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
		orm.GormLog(logger.NewGormLogger()), // 此处注意,日志需要先实例化
	)

mdb = orm.NewMysql()

```
