package orm

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"net"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const defaultDialTimeout = 10 * time.Second
const defaultIdentifierMaxLength = 64

var (
	errNilSQLDB       = errors.New("orm/v2: nil *sql.DB")
	errAddressInvalid = errors.New("orm/v2: mysql address is required")
)

type Config struct {
	Name        string
	MySQL       MySQLConfig
	Pool        PoolConfig
	GORM        GORMConfig
	Dialect     MySQLDialectConfig
	StartupPing bool
}

// MySQLConfig describes driver-level connection settings.
// Addr takes precedence over Host and Port when both are set.
// Prefer the Option helpers so Addr/Host/Port precedence stays consistent.
type MySQLConfig struct {
	User                 string
	Password             string
	Net                  string
	Host                 string
	Port                 string
	Addr                 string
	Database             string
	Params               map[string]string
	ConnectionAttributes string
	Collation            string
	Loc                  *time.Location
	TLSConfig            string
	Timeout              time.Duration
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	ParseTime            bool
}

type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	hasMaxOpenConns    bool
	hasMaxIdleConns    bool
	hasConnMaxLifetime bool
	hasConnMaxIdleTime bool
}

type GORMConfig struct {
	Logger                                   gormlogger.Interface
	NowFunc                                  func() time.Time
	NamingStrategy                           schema.NamingStrategy
	DefaultTransactionTimeout                time.Duration
	DefaultContextTimeout                    time.Duration
	PrepareStmt                              bool
	PrepareStmtMaxSize                       int
	PrepareStmtTTL                           time.Duration
	SkipDefaultTransaction                   bool
	DisableForeignKeyConstraintWhenMigrating bool
	IgnoreRelationshipsWhenMigrating         bool
	DisableNestedTransaction                 bool
	AllowGlobalUpdate                        bool
	QueryFields                              bool
	CreateBatchSize                          int
	TranslateError                           bool
	PropagateUnscoped                        bool
	DryRun                                   bool
}

type MySQLDialectConfig struct {
	DriverName                    string
	ServerVersion                 string
	DefaultStringSize             uint
	DefaultDatetimePrecision      *int
	SkipInitializeWithVersion     bool
	DisableWithReturning          bool
	DisableDatetimePrecision      bool
	DontSupportRenameIndex        bool
	DontSupportRenameColumn       bool
	DontSupportForShareClause     bool
	DontSupportNullAsDefaultValue bool
	DontSupportRenameColumnUnique bool
	DontSupportDropConstraint     bool
}

func DefaultConfig() Config {
	return Config{
		MySQL: MySQLConfig{
			Net:       "tcp",
			Host:      "127.0.0.1",
			Port:      "3306",
			Loc:       time.Local,
			Timeout:   defaultDialTimeout,
			ParseTime: true,
			Params: map[string]string{
				"charset": "utf8mb4",
			},
		},
		GORM: GORMConfig{
			NamingStrategy: defaultNamingStrategy(),
		},
		StartupPing: true,
	}
}

func NewConfig(opts ...Option) Config {
	return DefaultConfig().With(opts...)
}

func (c Config) With(opts ...Option) Config {
	clone := c.Clone()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&clone)
	}
	return clone
}

func (c Config) Clone() Config {
	clone := c
	clone.MySQL.Params = cloneStringMap(c.MySQL.Params)
	return clone
}

func (c Config) Open(ctx context.Context) (*Client, error) {
	driverCfg, err := c.DriverConfig()
	if err != nil {
		return nil, err
	}

	connector, err := mysqldriver.NewConnector(driverCfg)
	if err != nil {
		return nil, err
	}

	sqlDB := sql.OpenDB(connector)
	client, err := c.openWithSQLDB(ctx, sqlDB, true, driverCfg)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return client, nil
}

func (c Config) MustOpen(ctx context.Context) *Client {
	client, err := c.Open(ctx)
	if err != nil {
		panic(err)
	}
	return client
}

// OpenWithDB wraps an existing *sql.DB.
// Pool settings from Config.Pool are applied to sqlDB before GORM initialization.
// The caller retains ownership of sqlDB regardless of success or failure.
func (c Config) OpenWithDB(ctx context.Context, sqlDB *sql.DB) (*Client, error) {
	if sqlDB == nil {
		return nil, errNilSQLDB
	}
	return c.openWithSQLDB(ctx, sqlDB, false, nil)
}

func Open(ctx context.Context, opts ...Option) (*Client, error) {
	return NewConfig(opts...).Open(ctx)
}

func MustOpen(ctx context.Context, opts ...Option) *Client {
	return NewConfig(opts...).MustOpen(ctx)
}

// OpenWithDB wraps an existing *sql.DB.
// Pool settings from the supplied options are applied to sqlDB before GORM initialization.
// The caller retains ownership of sqlDB regardless of success or failure.
func OpenWithDB(ctx context.Context, sqlDB *sql.DB, opts ...Option) (*Client, error) {
	return NewConfig(opts...).OpenWithDB(ctx, sqlDB)
}

func (c Config) DriverConfig() (*mysqldriver.Config, error) {
	mysqlCfg := c.MySQL
	if mysqlCfg.Net == "" {
		mysqlCfg.Net = "tcp"
	}

	addr, err := mysqlCfg.address()
	if err != nil {
		return nil, err
	}

	cfg := mysqldriver.NewConfig()
	cfg.User = mysqlCfg.User
	cfg.Passwd = mysqlCfg.Password
	cfg.Net = mysqlCfg.Net
	cfg.Addr = addr
	cfg.DBName = mysqlCfg.Database
	cfg.Params = cloneStringMap(mysqlCfg.Params)
	cfg.ConnectionAttributes = mysqlCfg.ConnectionAttributes
	cfg.Collation = mysqlCfg.Collation
	cfg.Loc = mysqlCfg.Loc
	cfg.TLSConfig = mysqlCfg.TLSConfig
	cfg.Timeout = mysqlCfg.Timeout
	cfg.ReadTimeout = mysqlCfg.ReadTimeout
	cfg.WriteTimeout = mysqlCfg.WriteTimeout
	cfg.ParseTime = mysqlCfg.ParseTime
	return cfg, nil
}

func (c Config) RedactedDSN() (string, error) {
	driverCfg, err := c.DriverConfig()
	if err != nil {
		return "", err
	}
	if driverCfg.Passwd != "" {
		driverCfg.Passwd = "******"
	}
	return driverCfg.FormatDSN(), nil
}

func (c Config) openWithSQLDB(
	ctx context.Context,
	sqlDB *sql.DB,
	ownsSQLDB bool,
	driverCfg *mysqldriver.Config,
) (*Client, error) {
	clone := c.Clone()
	applyPoolConfig(sqlDB, clone.Pool)

	if clone.StartupPing {
		if err := sqlDB.PingContext(normalizeContext(ctx)); err != nil {
			return nil, err
		}
	}

	gdb, err := gorm.Open(gormmysql.New(clone.dialectorConfig(sqlDB, driverCfg)), clone.gormConfig())
	if err != nil {
		return nil, err
	}

	return &Client{
		db:        gdb,
		sqlDB:     sqlDB,
		config:    clone,
		ownsSQLDB: ownsSQLDB,
	}, nil
}

func (c Config) gormConfig() *gorm.Config {
	naming := c.GORM.NamingStrategy
	if naming.IdentifierMaxLength == 0 {
		naming.IdentifierMaxLength = defaultNamingStrategy().IdentifierMaxLength
	}

	return &gorm.Config{
		SkipDefaultTransaction:                   c.GORM.SkipDefaultTransaction,
		DefaultTransactionTimeout:                c.GORM.DefaultTransactionTimeout,
		DefaultContextTimeout:                    c.GORM.DefaultContextTimeout,
		NamingStrategy:                           naming,
		Logger:                                   c.GORM.Logger,
		NowFunc:                                  c.GORM.NowFunc,
		DryRun:                                   c.GORM.DryRun,
		PrepareStmt:                              c.GORM.PrepareStmt,
		PrepareStmtMaxSize:                       c.GORM.PrepareStmtMaxSize,
		PrepareStmtTTL:                           c.GORM.PrepareStmtTTL,
		DisableAutomaticPing:                     true,
		DisableForeignKeyConstraintWhenMigrating: c.GORM.DisableForeignKeyConstraintWhenMigrating,
		IgnoreRelationshipsWhenMigrating:         c.GORM.IgnoreRelationshipsWhenMigrating,
		DisableNestedTransaction:                 c.GORM.DisableNestedTransaction,
		AllowGlobalUpdate:                        c.GORM.AllowGlobalUpdate,
		QueryFields:                              c.GORM.QueryFields,
		CreateBatchSize:                          c.GORM.CreateBatchSize,
		TranslateError:                           c.GORM.TranslateError,
		PropagateUnscoped:                        c.GORM.PropagateUnscoped,
	}
}

func (c Config) dialectorConfig(sqlDB *sql.DB, driverCfg *mysqldriver.Config) gormmysql.Config {
	cfg := gormmysql.Config{
		DriverName:                    c.Dialect.DriverName,
		ServerVersion:                 c.Dialect.ServerVersion,
		Conn:                          sqlDB,
		SkipInitializeWithVersion:     c.Dialect.SkipInitializeWithVersion,
		DefaultStringSize:             c.Dialect.DefaultStringSize,
		DefaultDatetimePrecision:      c.Dialect.DefaultDatetimePrecision,
		DisableWithReturning:          c.Dialect.DisableWithReturning,
		DisableDatetimePrecision:      c.Dialect.DisableDatetimePrecision,
		DontSupportRenameIndex:        c.Dialect.DontSupportRenameIndex,
		DontSupportRenameColumn:       c.Dialect.DontSupportRenameColumn,
		DontSupportForShareClause:     c.Dialect.DontSupportForShareClause,
		DontSupportNullAsDefaultValue: c.Dialect.DontSupportNullAsDefaultValue,
		DontSupportRenameColumnUnique: c.Dialect.DontSupportRenameColumnUnique,
		DontSupportDropConstraint:     c.Dialect.DontSupportDropConstraint,
	}

	if driverCfg != nil {
		cfg.DSNConfig = driverCfg.Clone()
	}

	return cfg
}

func applyPoolConfig(sqlDB *sql.DB, pool PoolConfig) {
	if pool.hasMaxOpenConns {
		sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	}
	if pool.hasMaxIdleConns {
		sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	}
	if pool.hasConnMaxLifetime {
		sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime)
	}
	if pool.hasConnMaxIdleTime {
		sqlDB.SetConnMaxIdleTime(pool.ConnMaxIdleTime)
	}
}

func defaultNamingStrategy() schema.NamingStrategy {
	return schema.NamingStrategy{
		IdentifierMaxLength: defaultIdentifierMaxLength,
	}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func cloneStringMap(src map[string]string) map[string]string {
	return maps.Clone(src)
}

func (c MySQLConfig) address() (string, error) {
	if c.Addr != "" {
		return c.Addr, nil
	}
	if c.Host == "" || c.Port == "" {
		return "", errAddressInvalid
	}
	return net.JoinHostPort(c.Host, c.Port), nil
}
