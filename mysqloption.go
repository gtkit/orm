package orm

import "time"

type options struct {
	DbType   string
	Host     string
	Port     string
	DbName   string
	Username string
	Password string

	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration

	hasMaxOpenConns    bool
	hasMaxIdleConns    bool
	hasConnMaxLifetime bool
	hasConnMaxIdleTime bool
}

type Options interface {
	apply(*options)
}

type dbtype struct {
	dbtype string
}

func (db dbtype) apply(opt *options) {
	opt.DbType = db.dbtype
}

func DbType(t string) Options {
	return dbtype{dbtype: t}
}

type host struct {
	host string
}

func (h host) apply(opt *options) {
	opt.Host = h.host
}

func Host(h string) Options {
	return host{host: h}
}

type port struct {
	port string
}

func (p port) apply(opt *options) {
	opt.Port = p.port
}

func Port(p string) Options {
	return port{port: p}
}

type name struct {
	name string
}

func (n name) apply(opt *options) {
	opt.DbName = n.name
}

func Name(n string) Options {
	return name{name: n}
}

type username struct {
	username string
}

func (u username) apply(opt *options) {
	opt.Username = u.username
}

func User(u string) Options {
	return username{username: u}
}

type password struct {
	password string
}

func (p password) apply(opt *options) {
	opt.Password = p.password
}

func WithPassword(pass string) Options {
	return password{password: pass}
}

type maxOpenConns struct {
	size int
}

func (m maxOpenConns) apply(opt *options) {
	opt.maxOpenConns = m.size
	opt.hasMaxOpenConns = true
}

func MaxOpenConns(size int) Options {
	return maxOpenConns{size: size}
}

type maxIdleConns struct {
	size int
}

func (m maxIdleConns) apply(opt *options) {
	opt.maxIdleConns = m.size
	opt.hasMaxIdleConns = true
}

func MaxIdleConns(size int) Options {
	return maxIdleConns{size: size}
}

type connMaxLifetime struct {
	duration time.Duration
}

func (c connMaxLifetime) apply(opt *options) {
	opt.connMaxLifetime = c.duration
	opt.hasConnMaxLifetime = true
}

func ConnMaxLifetime(duration time.Duration) Options {
	return connMaxLifetime{duration: duration}
}

type connMaxIdleTime struct {
	duration time.Duration
}

func (c connMaxIdleTime) apply(opt *options) {
	opt.connMaxIdleTime = c.duration
	opt.hasConnMaxIdleTime = true
}

func ConnMaxIdleTime(duration time.Duration) Options {
	return connMaxIdleTime{duration: duration}
}
