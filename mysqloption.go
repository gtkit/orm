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
	maxopenconn,
	maxidleconn int
	maxlifeseconds time.Duration
}
type Options interface {
	apply(*options)
}

type dbtype struct {
	dbtype string
}

func (db dbtype) apply(opt *options) {
	opt.DbType = opt.DbType
}

// 数据库类型配置.
func DbType(t string) Options {
	return dbtype{dbtype: t}
}

type host struct {
	host string
}

func (h host) apply(opt *options) {
	opt.Host = h.host
}

// 数据库链接地址配置.
func Host(h string) Options {
	return host{host: h}
}

type port struct {
	port string
}

func (p port) apply(opt *options) {
	opt.Port = p.port
}

// 数据库端口配置.
func Port(p string) Options {
	return port{port: p}
}

type name struct {
	name string
}

func (n name) apply(opt *options) {
	opt.DbName = n.name
}

// 数据库名称配置.
func Name(n string) Options {
	return name{name: n}
}

type username struct {
	username string
}

func (u username) apply(opt *options) {
	opt.Username = u.username
}

// 登录用户名配置.
func User(u string) Options {
	return username{username: u}
}

type password struct {
	password string
}

func (p password) apply(opt *options) {
	opt.Password = p.password
}

// 登录密码配置.
func WithPassword(pass string) Options {
	return password{password: pass}
}
