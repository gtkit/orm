// @Author xiaozhaofu 2023/1/10 17:03:00
package orm

import (
	"time"
)

type options struct {
	DbType, // 数据库类型
	Host, // 数据库链接地址
	Port, // 端口号
	DbName, // 数据库名称
	Username, // 登录用户名
	Password string // 登录密码

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

// DbType 数据库类型配置.
func DbType(t string) Options {
	return dbtype{dbtype: t}
}

type host struct {
	host string
}

func (h host) apply(opt *options) {
	opt.Host = h.host
}

// Host 数据库链接地址配置.
func Host(h string) Options {
	return host{host: h}
}

type port struct {
	port string
}

func (p port) apply(opt *options) {
	opt.Port = p.port
}

// Port 数据库端口配置.
func Port(p string) Options {
	return port{port: p}
}

type name struct {
	name string
}

func (n name) apply(opt *options) {
	opt.DbName = n.name
}

// Name 数据库名称配置.
func Name(n string) Options {
	return name{name: n}
}

type username struct {
	username string
}

func (u username) apply(opt *options) {
	opt.Username = u.username
}

// User 登录用户名配置.
func User(u string) Options {
	return username{username: u}
}

type password struct {
	password string
}

func (p password) apply(opt *options) {
	opt.Password = p.password
}

// WithPassword 登录密码配置.
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

// MaxOpenConns 数据库最大打开连接数配置.
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

// MaxIdleConns 数据库最大空闲连接数配置.
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

// ConnMaxLifetime 数据库连接最大存活时间配置.
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

// ConnMaxIdleTime 数据库连接最大空闲时间配置.
func ConnMaxIdleTime(duration time.Duration) Options {
	return connMaxIdleTime{duration: duration}
}
