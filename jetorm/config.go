package jetorm

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const (
	defaultDriver          = "mysql"
	defaultHost            = "127.0.0.1"
	defaultPort            = "3306"
	defaultMaxOpenConns    = 50
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 10 * time.Minute
	defaultDialTimeout     = 10 * time.Second
	defaultReadTimeout     = 30 * time.Second
	defaultWriteTimeout    = 30 * time.Second
)

var (
	ErrNilDB     = errors.New("jetorm: db is required")
	ErrNilTxFunc = errors.New("jetorm: tx func is required")
	openDBFn     = openDB
)

type Config struct {
	Driver          string
	Host            string
	Port            string
	Database        string
	User            string
	Password        string `json:"-"`
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	QueryTimeout    time.Duration
}

type Option interface {
	apply(*Config)
}

type optionFunc func(*Config)

func (f optionFunc) apply(cfg *Config) {
	f(cfg)
}

func NewConfig(opts ...Option) Config {
	cfg := Config{
		Driver:          defaultDriver,
		Host:            defaultHost,
		Port:            defaultPort,
		MaxOpenConns:    defaultMaxOpenConns,
		MaxIdleConns:    defaultMaxIdleConns,
		ConnMaxLifetime: defaultConnMaxLifetime,
		ConnMaxIdleTime: defaultConnMaxIdleTime,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}
	return cfg
}

func (c Config) Clone() Config {
	return c
}

func (c Config) DSN() string {
	cfg := c.driverConfig()
	return cfg.FormatDSN()
}

func (c Config) RedactedDSN() string {
	cfg := c.driverConfig()
	if cfg.Passwd != "" {
		cfg.Passwd = "******"
	}
	return cfg.FormatDSN()
}

func (c Config) driverConfig() *mysqldriver.Config {
	cfg := mysqldriver.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.Host, c.Port)
	cfg.DBName = c.Database
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Timeout = defaultDialTimeout
	cfg.ReadTimeout = defaultReadTimeout
	cfg.WriteTimeout = defaultWriteTimeout
	return cfg
}

func openDB(cfg Config) (*sql.DB, error) {
	return sql.Open(cfg.Driver, cfg.DSN())
}

func applyPoolOptions(db *sql.DB, cfg Config) {
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
}

func normalizeContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func WithHost(host string) Option {
	return optionFunc(func(cfg *Config) { cfg.Host = host })
}

func WithPort(port string) Option {
	return optionFunc(func(cfg *Config) { cfg.Port = port })
}

func WithDatabase(name string) Option {
	return optionFunc(func(cfg *Config) { cfg.Database = name })
}

func WithUser(user string) Option {
	return optionFunc(func(cfg *Config) { cfg.User = user })
}

func WithPassword(password string) Option {
	return optionFunc(func(cfg *Config) { cfg.Password = password })
}

func WithMaxOpenConns(n int) Option {
	return optionFunc(func(cfg *Config) { cfg.MaxOpenConns = n })
}

func WithMaxIdleConns(n int) Option {
	return optionFunc(func(cfg *Config) { cfg.MaxIdleConns = n })
}

func WithConnMaxLifetime(d time.Duration) Option {
	return optionFunc(func(cfg *Config) { cfg.ConnMaxLifetime = d })
}

func WithConnMaxIdleTime(d time.Duration) Option {
	return optionFunc(func(cfg *Config) { cfg.ConnMaxIdleTime = d })
}

func WithQueryTimeout(d time.Duration) Option {
	return optionFunc(func(cfg *Config) { cfg.QueryTimeout = d })
}
