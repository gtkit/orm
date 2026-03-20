package orm

import (
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type GormOptions interface {
	apply(config *gorm.Config)
}

type preparestmt struct {
	preparestmt bool
}

func (p preparestmt) apply(conf *gorm.Config) {
	conf.PrepareStmt = p.preparestmt
}

// PrepareStmt preparestmt 配置是否预编译sql语句.
func PrepareStmt(prepare bool) GormOptions {
	return preparestmt{preparestmt: prepare}
}

type skipdefaulttransaction struct {
	skipdefaulttransaction bool
}

func (s skipdefaulttransaction) apply(conf *gorm.Config) {
	conf.SkipDefaultTransaction = s.skipdefaulttransaction
}

// SkipDefaultTransaction skipdefaulttransaction 配置是否跳过默认事务.
func SkipDefaultTransaction(skip bool) GormOptions {
	return skipdefaulttransaction{skipdefaulttransaction: skip}
}

type log struct {
	logger gormlogger.Interface
}

func (l log) apply(conf *gorm.Config) {
	conf.Logger = l.logger
}

// GormLog log 配置日志.
func GormLog(l gormlogger.Interface) GormOptions {
	return log{logger: l}
}

type nowfunc struct {
	nowfunc func() time.Time
}

func (n nowfunc) apply(conf *gorm.Config) {
	conf.NowFunc = n.nowfunc
}

// NowFunc nowfunc 配置自定义now函数.
func NowFunc(f func() time.Time) GormOptions {
	return nowfunc{nowfunc: f}
}

func defaultNamingStrategy() schema.NamingStrategy {
	return schema.NamingStrategy{
		TablePrefix:         "",
		SingularTable:       false,
		NameReplacer:        nil,
		NoLowerCase:         false,
		IdentifierMaxLength: 64,
	}
}

func defaultGormConfig() gorm.Config {
	return gorm.Config{
		NamingStrategy: defaultNamingStrategy(),
	}
}

func currentNamingStrategy(conf *gorm.Config) schema.NamingStrategy {
	if conf == nil {
		return defaultNamingStrategy()
	}
	switch ns := conf.NamingStrategy.(type) {
	case schema.NamingStrategy:
		return ns
	case *schema.NamingStrategy:
		if ns != nil {
			return *ns
		}
	}
	return defaultNamingStrategy()
}

type tablePrefix struct {
	tablePrefix string
}

func (t tablePrefix) apply(conf *gorm.Config) {
	strategy := currentNamingStrategy(conf)
	strategy.TablePrefix = t.tablePrefix
	conf.NamingStrategy = strategy
}

// TablePrefix tablePrefix 配置表名前缀.
func TablePrefix(prefix string) GormOptions {
	return tablePrefix{tablePrefix: prefix}
}

type singularTable struct {
	singularTable bool
}

func (s singularTable) apply(conf *gorm.Config) {
	strategy := currentNamingStrategy(conf)
	strategy.SingularTable = s.singularTable
	conf.NamingStrategy = strategy
}

// SingularTable singularTable 配置是否使用单数表名.
func SingularTable(singular bool) GormOptions {
	return singularTable{singularTable: singular}
}
