package orm

import (
	"net"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	configMu sync.RWMutex
	mop      = defaultMysqlOptions()
	gop      = defaultGormConfig()
)

const defaultDialTimeout = 10 * time.Second

func defaultMysqlOptions() options {
	return options{
		DbType:   "mysql",
		Username: "root",
		Password: "",
		Host:     "127.0.0.1",
		Port:     "3306",
	}
}

func NewMysql(setter ...Setter) *gorm.DB {
	db, err := OpenMysql(setter...)
	if err != nil {
		panic(err)
	}
	return db
}

func OpenMysql(setter ...Setter) (*gorm.DB, error) {
	mydb := new(Mysql)
	mysqlOpts := mysqlOptionsSnapshot()
	db, err := mydb.open(buildMySQLDSN(mysqlOpts), gormConfigSnapshot())
	if err != nil {
		return nil, err
	}

	if db.Error != nil {
		return nil, db.Error
	}

	if applyErr := applyPoolOptions(db, mysqlOpts); applyErr != nil {
		return nil, applyErr
	}

	applySetters(db, setter)
	return db, nil
}

type Setter interface {
	// Set is applied after connection pool options.
	// Implementations should avoid mutating the underlying *sql.DB pool settings;
	// use MysqlConfig(MaxOpenConns(...)) and related options instead.
	Set(db *gorm.DB)
}

type Mysql struct{}

func MysqlConfig(opts ...Options) {
	configMu.Lock()
	defer configMu.Unlock()

	mop = defaultMysqlOptions()
	for _, option := range opts {
		if option == nil {
			continue
		}
		option.apply(&mop)
	}
}

func GormConfig(opts ...GormOptions) {
	configMu.Lock()
	defer configMu.Unlock()

	gop = defaultGormConfig()
	for _, option := range opts {
		if option == nil {
			continue
		}
		option.apply(&gop)
	}
}

func (e *Mysql) GetConnect() string {
	return buildMySQLDSN(mysqlOptionsSnapshot())
}

func (e *Mysql) Open(conn string) (*gorm.DB, error) {
	return e.open(conn, gormConfigSnapshot())
}

func (e *Mysql) open(conn string, conf gorm.Config) (*gorm.DB, error) {
	return gorm.Open(gormmysql.Open(conn), &conf)
}

func mysqlOptionsSnapshot() options {
	configMu.RLock()
	defer configMu.RUnlock()

	return mop
}

func gormConfigSnapshot() gorm.Config {
	configMu.RLock()
	defer configMu.RUnlock()

	return cloneGormConfig(gop)
}

func cloneGormConfig(conf gorm.Config) gorm.Config {
	clone := conf
	clone.NamingStrategy = currentNamingStrategy(&conf)

	if conf.ClauseBuilders != nil {
		clone.ClauseBuilders = make(map[string]clause.ClauseBuilder, len(conf.ClauseBuilders))
		for key, builder := range conf.ClauseBuilders {
			clone.ClauseBuilders[key] = builder
		}
	}

	if conf.Plugins != nil {
		clone.Plugins = make(map[string]gorm.Plugin, len(conf.Plugins))
		for key, plugin := range conf.Plugins {
			clone.Plugins[key] = plugin
		}
	}

	return clone
}

func buildMySQLDSN(opt options) string {
	cfg := mysqldriver.NewConfig()
	cfg.User = opt.Username
	cfg.Passwd = opt.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(opt.Host, opt.Port)
	cfg.DBName = opt.DbName
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Timeout = defaultDialTimeout
	cfg.Params = map[string]string{
		"charset": "utf8mb4",
	}

	return cfg.FormatDSN()
}

func applyPoolOptions(db *gorm.DB, opt options) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	if opt.hasMaxOpenConns {
		sqlDB.SetMaxOpenConns(opt.maxOpenConns)
	}
	if opt.hasMaxIdleConns {
		sqlDB.SetMaxIdleConns(opt.maxIdleConns)
	}
	if opt.hasConnMaxLifetime {
		sqlDB.SetConnMaxLifetime(opt.connMaxLifetime)
	}
	if opt.hasConnMaxIdleTime {
		sqlDB.SetConnMaxIdleTime(opt.connMaxIdleTime)
	}

	return nil
}

func applySetters(db *gorm.DB, setters []Setter) {
	for _, setter := range setters {
		if setter == nil {
			continue
		}
		setter.Set(db)
	}
}
