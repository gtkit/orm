// Package orm provides a GORM-based MySQL connection wrapper.
//
// For single-node use cases, v1 provides a simple global-config API.
// For cluster failover, health checks, and per-instance configuration,
// see [github.com/gtkit/orm/v2].
package orm

import (
	"fmt"
	"net"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	configMu           sync.RWMutex
	mop                = defaultMysqlOptions()
	gop                = defaultGormConfig()
	mysqlOpenFn        = func(m *Mysql, conn string, conf gorm.Config) (*gorm.DB, error) { return m.open(conn, conf) }
	applyPoolOptionsFn = applyPoolOptions
)

const (
	defaultDialTimeout  = 10 * time.Second
	defaultReadTimeout  = 30 * time.Second
	defaultWriteTimeout = 30 * time.Second
)

func defaultMysqlOptions() options {
	return options{
		DbType:             "mysql",
		Username:           "root",
		Password:           "",
		Host:               "127.0.0.1",
		Port:               "3306",
		maxOpenConns:       defaultMaxOpenConns,
		maxIdleConns:       defaultMaxIdleConns,
		connMaxLifetime:    defaultConnMaxLifetime,
		connMaxIdleTime:    defaultConnMaxIdleTime,
		hasMaxOpenConns:    true,
		hasMaxIdleConns:    true,
		hasConnMaxLifetime: true,
		hasConnMaxIdleTime: true,
	}
}

// NewMysql creates a GORM DB instance or panics on failure.
// Use only during application startup (e.g. in main or init).
// For production services that need graceful error handling, prefer [OpenMysql].
func NewMysql(setter ...Setter) *gorm.DB {
	db, err := OpenMysql(setter...)
	if err != nil {
		panic(err)
	}
	return db
}

// DBResult holds the GORM DB and the underlying *sql.DB for lifecycle management.
type DBResult struct {
	DB    *gorm.DB
	SQLDB interface{ Close() error }
}

// Close closes the underlying database connection.
func (r *DBResult) Close() error {
	if r.SQLDB == nil {
		return nil
	}
	return r.SQLDB.Close()
}

// OpenMysqlWithClose creates a GORM DB and returns a DBResult that allows
// the caller to close the underlying connection via defer result.Close().
func OpenMysqlWithClose(setter ...Setter) (*DBResult, error) {
	mydb := new(Mysql)
	mysqlOpts := mysqlOptionsSnapshot()
	db, err := mysqlOpenFn(mydb, buildMySQLDSN(mysqlOpts), gormConfigSnapshot())
	if err != nil {
		return nil, err
	}

	if db.Error != nil {
		return nil, db.Error
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if applyErr := applyPoolOptionsFn(db, mysqlOpts); applyErr != nil {
		_ = sqlDB.Close()
		return nil, applyErr
	}

	applySetters(db, setter)
	return &DBResult{DB: db, SQLDB: sqlDB}, nil
}

func OpenMysql(setter ...Setter) (*gorm.DB, error) {
	mydb := new(Mysql)
	mysqlOpts := mysqlOptionsSnapshot()
	db, err := mysqlOpenFn(mydb, buildMySQLDSN(mysqlOpts), gormConfigSnapshot())
	if err != nil {
		return nil, err
	}

	if db.Error != nil {
		return nil, db.Error
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if applyErr := applyPoolOptionsFn(db, mysqlOpts); applyErr != nil {
		_ = sqlDB.Close()
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

// RedactedDSN returns the DSN string with the password replaced by "******".
// Use this for logging or debugging — never log the raw DSN.
func RedactedDSN() string {
	opts := mysqlOptionsSnapshot()
	cfg := buildMySQLDriverConfig(opts)
	if cfg.Passwd != "" {
		cfg.Passwd = "******"
	}
	return cfg.FormatDSN()
}

// String returns a redacted connection description for safe logging.
func (opt options) String() string {
	return fmt.Sprintf("orm.Config{user=%s, host=%s:%s, db=%s, password=******}",
		opt.Username, opt.Host, opt.Port, opt.DbName)
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

func buildMySQLDriverConfig(opt options) *mysqldriver.Config {
	cfg := mysqldriver.NewConfig()
	cfg.User = opt.Username
	cfg.Passwd = opt.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(opt.Host, opt.Port)
	cfg.DBName = opt.DbName
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Timeout = defaultDialTimeout
	cfg.ReadTimeout = defaultReadTimeout
	cfg.WriteTimeout = defaultWriteTimeout
	return cfg
}

func buildMySQLDSN(opt options) string {
	return buildMySQLDriverConfig(opt).FormatDSN()
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
