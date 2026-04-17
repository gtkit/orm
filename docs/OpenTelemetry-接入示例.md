# OpenTelemetry 接入示例

本文档给出 `orm/v2` 在不直接绑定 OTel 依赖的前提下，如何做链路追踪接入。

## 推荐策略

`orm/v2` 负责：

- 连接管理
- 路由
- 事务重试观测
- trace_id 日志关联

业务接入层负责：

- span 创建
- trace exporter 配置
- Prometheus / OTel 指标后端集成

## 方案一：在事务 observer 中记录事件

如果你的重点是观察死锁重试，可直接用 `WithTxRetryObserver(...)` 往 span 里打事件：

```go
client, err := orm.Open(
	ctx,
	orm.WithTxRetryObserver(func(ctx context.Context, event orm.TxRetryEvent) {
		span := trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return
		}
		span.AddEvent("orm.tx.retry",
			trace.WithAttributes(
				attribute.String("db.client_name", event.ClientName),
				attribute.Int("db.retry_attempt", event.Attempt),
				attribute.Int("db.max_retries", event.MaxRetries),
				attribute.String("db.wait", event.Wait.String()),
			),
		)
	}),
)
```

## 方案二：配合 GORM / SQL 层的 OTel 插件

如果你需要每条 SQL 的 span，而不是只关心重试事件，建议在接入层叠加：

- `otelsql`
- 或 GORM 生态中的 tracing 插件

示意：

```go
client, err := orm.Open(ctx, orm.WithHost("127.0.0.1"), ...)
if err != nil {
	return err
}

db := client.DB()

// 在这里注册你团队统一采用的 GORM / SQL tracing 插件
// 例如：
// db.Use(myTracingPlugin)
```

## 与日志关联

如果你已经在请求上下文里放入 trace id，可以继续配合 `zlogger`：

```go
logger := ormzap.New(
	ormzap.WithTraceIDExtractor(func(ctx context.Context) string {
		spanCtx := trace.SpanContextFromContext(ctx)
		if !spanCtx.IsValid() {
			return ""
		}
		return spanCtx.TraceID().String()
	}),
)
```

这样 SQL 日志会带上 `trace_id`，即使你没有给每条 SQL 建 span，也能先把日志链路串起来。
