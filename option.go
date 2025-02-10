// @Author xiaozhaofu 2023/1/10 17:03:00
package orm

import (
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type options struct {
	DbType, // 数据库类型
	Host, // 数据库链接地址
	Port, // 端口号
	DbName, // 数据库名称
	Username, // 登录用户名
	Password string // 登录密码
	maxopenconn,
	maxidleconn int
	maxlifeseconds time.Duration
}
type Options interface {
	apply(*options)
}
type GormOptions interface {
	apply(config *gorm.Config)
}
type dbtype struct {
	dbtype string
}

func (db dbtype) apply(opt *options) {
	opt.DbType = opt.DbType
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

/**
 * Gorm 配置
 */

// preparestmt 配置是否预编译sql语句.
type preparestmt struct {
	preparestmt bool
}

func (p preparestmt) apply(conf *gorm.Config) {
	conf.PrepareStmt = p.preparestmt
}

func PrepareStmt(prepare bool) GormOptions {
	return preparestmt{preparestmt: prepare}
}

// skipdefaulttransaction 配置是否跳过默认事务.
type skipdefaulttransaction struct {
	skipdefaulttransaction bool
}

func (s skipdefaulttransaction) apply(conf *gorm.Config) {
	conf.SkipDefaultTransaction = s.skipdefaulttransaction
}
func SkipDefaultTransaction(skip bool) GormOptions {
	return skipdefaulttransaction{skipdefaulttransaction: skip}
}

// log 配置日志.
type log struct {
	logger gormlogger.Interface
}

func (l log) apply(conf *gorm.Config) {
	conf.Logger = l.logger
}
func GormLog(l gormlogger.Interface) GormOptions {
	return log{logger: l}
}

// nowfunc 配置自定义now函数.
type nowfunc struct {
	nowfunc func() time.Time
}

func (n nowfunc) apply(conf *gorm.Config) {
	conf.NowFunc = n.nowfunc
}
func NowFunc(f func() time.Time) GormOptions {
	return nowfunc{nowfunc: f}
}
