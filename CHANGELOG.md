# 变更记录

本文档记录 `github.com/gtkit/orm` 的对外可见变更。

格式参考 Keep a Changelog，版本遵循语义化版本。

## [未发布]

### 新增

- `orm/v2` 新增自定义健康探针 `WithHealthProbe(...)`
- `orm/v2` 新增显式健康循环 `RunHealthLoop(ctx, interval)`
- `orm/v2` 新增写后读窗口与清除语义：
  - `ContextWithWriteWindow(...)`
  - `ContextClearWriteFlag(...)`
- `orm/v2` 新增启动期探活重试 `WithStartupPingRetry(...)`
- `orm/v2` 新增事务重试观测 `WithTxRetryObserver(...)`
- `orm/v2` 新增事务选项 `WithRetryMaxWait(...)`
- 新增 GitHub Actions CI 门禁
- 新增真实 MySQL 集成测试
- 新增 Prometheus / OpenTelemetry 接入参考文档

### 变更

- `orm` / `orm/v2` 默认不再强制写入 `charset` DSN 参数，避免在新版本 MySQL 上触发兼容性问题
- `v1` 进入维护模式，新增能力仅进入 `v2`

## [2026-04-17]

### 新增

- 初始发布本仓库内的企业级增强与 CI 体系
