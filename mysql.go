package orm

import (
	"bytes"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	mop options
	gop gorm.Config
)

func NewMysql(setter ...Setter) *gorm.DB {
	mydb := new(Mysql)
	db, err := mydb.Open(mydb.GetConnect())

	if err != nil {
		// logger.Fatalf("%s connect error %v", mop.DbType, err)
		panic(err)
	}

	if db.Error != nil {
		// logger.Fatalf("database error %v", db.Error)
		panic(err)
	}
	// 自定义配置
	if len(setter) > 0 && setter[0] != nil {
		setter[0].Set(db)
	}

	return db
}

type Setter interface {
	Set(db *gorm.DB)
}

type Mysql struct{}

func MysqlConfig(opts ...Options) {
	mop = options{
		DbType:   "mysql",
		Username: "root",
		Password: "",
		Host:     "127.0.0.1",
		Port:     "3306",
	}
	for _, o := range opts {
		o.apply(&mop)
	}
}
func GormConfig(opts ...GormOptions) {
	for _, o := range opts {
		o.apply(&gop)
	}
	gop.NamingStrategy = gormNamingStrategy
}

func (e *Mysql) GetConnect() string {
	var conn bytes.Buffer // bytes.buffer是一个缓冲byte类型的缓冲器存放着都是byte
	conn.WriteString(mop.Username)
	conn.WriteString(":")
	conn.WriteString(mop.Password)
	conn.WriteString("@tcp(")
	conn.WriteString(mop.Host)
	conn.WriteString(":")
	conn.WriteString(mop.Port)
	conn.WriteString(")")
	conn.WriteString("/")
	conn.WriteString(mop.DbName)
	conn.WriteString("?charset=utf8mb4&parseTime=True&loc=Local&timeout=10000ms")
	return conn.String()
}

func (e *Mysql) Open(conn string) (db *gorm.DB, err error) {
	return gorm.Open(mysql.Open(conn), &gop)
}
