# orm/v2 Phase 1 Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `orm/v2` 补齐第一阶段企业级增强能力，包括健康探活循环、自定义健康探针、write flag 清除/窗口语义、事务重试观测钩子、启动期 ping retry，以及对应文档。

**Architecture:** 保持 `orm/v2` 作为“连接管理库”边界不变，不把它扩展成查询执行代理。新增能力统一走显式 API、Option 或 Hook；不引入隐式后台 goroutine，也不把 Prometheus/OTel 强耦合进 core。文档层补充迁移、风险与行为说明。

**Tech Stack:** Go 1.26, GORM, go-sql-driver/mysql, GitHub Actions, `testing`

---

### Task 1: 健康探针抽象

**Files:**
- Modify: `v2/health.go`
- Modify: `v2/config.go`
- Modify: `v2/options.go`
- Test: `v2/client_ops_test.go`
- Test: `v2/cluster_test.go`

- [ ] **Step 1: 写失败测试**

为单节点和集群健康检查增加“自定义 probe 失败应返回 down”的测试，覆盖 `Client.HealthCheck()` 和 `Cluster.Refresh()`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd v2 && go test -run 'TestClientHealthCheckUsesCustomProbe|TestClusterRefreshUsesCustomProbe' ./...`

- [ ] **Step 3: 实现最小代码**

新增 `HealthProbeFunc`、`WithHealthProbe()`，在默认 `PingContext()` 成功后执行自定义 probe；自定义 probe 失败时将节点标记为 down。

- [ ] **Step 4: 运行目标测试确认通过**

Run: `cd v2 && go test -run 'TestClientHealthCheckUsesCustomProbe|TestClusterRefreshUsesCustomProbe' ./...`

- [ ] **Step 5: 运行关联回归**

Run: `cd v2 && go test -run 'TestClientHealthCheck|TestClusterHealthCheck|TestClusterRefresh' ./...`


### Task 2: 集群健康循环

**Files:**
- Modify: `v2/cluster.go`
- Test: `v2/cluster_test.go`
- Modify: `v2/README.md`

- [ ] **Step 1: 写失败测试**

增加 `RunHealthLoop(ctx, interval)` 行为测试，验证它会周期性触发 `Refresh()`，并在 `ctx.Done()` 后退出。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd v2 && go test -run 'TestClusterRunHealthLoop' ./...`

- [ ] **Step 3: 实现最小代码**

新增阻塞式 `RunHealthLoop(ctx, interval)`，内部用 `time.Ticker` 调度 `Refresh(ctx)`，不自动起 goroutine，`interval <= 0` 返回错误。

- [ ] **Step 4: 运行目标测试确认通过**

Run: `cd v2 && go test -run 'TestClusterRunHealthLoop' ./...`

- [ ] **Step 5: 文档化使用方式**

在 `v2/README.md` 增加 `go cluster.RunHealthLoop(ctx, 5*time.Second)` 示例，并明确“由调用方控制 goroutine 生命周期”。


### Task 3: write flag 清除与窗口期

**Files:**
- Modify: `v2/readafter.go`
- Test: `v2/readafter_test.go`
- Test: `v2/cluster_test.go`
- Modify: `v2/README.md`

- [ ] **Step 1: 写失败测试**

增加 `ContextClearWriteFlag()` 和 `ContextWithWriteWindow()` 的测试，验证：
- clear 后恢复副本路由
- TTL 过期后恢复副本路由

- [ ] **Step 2: 运行测试确认失败**

Run: `cd v2 && go test -run 'TestContextClearWriteFlag|TestContextWithWriteWindow|TestReaderClientCtxClearedWriteFlag|TestReaderClientCtxWriteWindowExpires' ./...`

- [ ] **Step 3: 实现最小代码**

将 write flag 从布尔值提升为内部状态结构，保留 `ContextWithWriteFlag()` 兼容语义，新增：
- `ContextClearWriteFlag(ctx context.Context) context.Context`
- `ContextWithWriteWindow(ctx context.Context, ttl time.Duration) context.Context`

- [ ] **Step 4: 运行目标测试确认通过**

Run: `cd v2 && go test -run 'TestContextClearWriteFlag|TestContextWithWriteWindow|TestReaderClientCtxClearedWriteFlag|TestReaderClientCtxWriteWindowExpires' ./...`

- [ ] **Step 5: 文档化长生命周期 context 风险**

在 `v2/README.md` 明确说明 websocket / consumer 不应复用永久 write flag。


### Task 4: 启动期 Ping Retry

**Files:**
- Modify: `v2/config.go`
- Modify: `v2/options.go`
- Test: `v2/config_test.go`
- Test: `v2/testhelper_test.go`
- Modify: `v2/README.md`

- [ ] **Step 1: 写失败测试**

增加“startup ping 首次失败但后续成功时，应按 retry 配置重试成功”的测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd v2 && go test -run 'TestOpenRetriesStartupPing' ./...`

- [ ] **Step 3: 实现最小代码**

为 `Config` 增加启动期 ping retry 配置，暴露 `WithStartupPingRetry(...)` Option，在 `openWithSQLDB()` 中按 backoff 重试。

- [ ] **Step 4: 运行目标测试确认通过**

Run: `cd v2 && go test -run 'TestOpenRetriesStartupPing' ./...`

- [ ] **Step 5: 回归验证**

Run: `cd v2 && go test -run 'TestOpenWithDBUsesExternalPool|TestOpenWithoutStartupPingDoesNotDialImmediately' ./...`


### Task 5: 事务重试观测钩子

**Files:**
- Modify: `v2/tx.go`
- Modify: `v2/config.go`
- Modify: `v2/options.go`
- Test: `v2/client_ops_test.go`
- Modify: `v2/README.md`

- [ ] **Step 1: 写失败测试**

增加“死锁重试发生时，observer 会收到 attempt / wait / err 信息”的测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd v2 && go test -run 'TestWithTxReportsRetryEvent' ./...`

- [ ] **Step 3: 实现最小代码**

新增 `TxRetryObserver` / `TxRetryEvent` 和 `WithTxRetryObserver()`；在 `withTxRetry()` 的 deadlock 分支中回调 observer。

- [ ] **Step 4: 运行目标测试确认通过**

Run: `cd v2 && go test -run 'TestWithTxReportsRetryEvent' ./...`

- [ ] **Step 5: 保持无 Prometheus/OTel 绑定**

只保留 hook，不直接引入监控依赖。


### Task 6: 文档与迁移说明

**Files:**
- Modify: `README.md`
- Modify: `v2/README.md`

- [ ] **Step 1: 更新根 README**

补充 v1 -> v2 迁移说明、建议新接入默认用 v2、v1 进入维护模式的时间表。

- [ ] **Step 2: 更新 v2 README**

补充以下内容：
- 健康循环示例
- 自定义健康探针示例
- `PrepareStmt=true` + 读写分离注意事项
- `WithTx()` panic 行为说明
- write flag 窗口期/清除示例
- 启动期 ping retry 示例

- [ ] **Step 3: 手动核对 README 前后一致性**

确认示例 API 与真实导出符号一致。


### Task 7: 全量验证

**Files:**
- Modify: 无

- [ ] **Step 1: 跑 v2 单元与 race**

Run:
```bash
cd v2
go test ./...
go test -race ./...
go vet ./...
```

- [ ] **Step 2: 跑 root 单元与 race**

Run:
```bash
go test ./...
go test -race ./...
go vet ./...
```

- [ ] **Step 3: 跑漏洞扫描**

Run:
```bash
govulncheck ./...
cd v2 && govulncheck ./...
```

- [ ] **Step 4: 跑 lint**

Run:
```bash
docker run --rm -v "$PWD":/app -w /app golangci/golangci-lint:v2.11.4 golangci-lint run --config .golangci.yml ./...
docker run --rm -v "$PWD":/app -w /app/v2 golangci/golangci-lint:v2.11.4 golangci-lint run --config ../.golangci.yml ./...
```

- [ ] **Step 5: 跑真实 MySQL 集成测试**

Run:
```bash
cd v2
ORM_RUN_INTEGRATION=1 ORM_TEST_DSN='root:root@tcp(127.0.0.1:3306)/' go test -run Integration ./...
```
