# 贡献指南

感谢你为 `github.com/gtkit/orm` 做贡献。

## 基本原则

- 所有新增说明、注释、文档优先使用中文
- 默认保持 `orm/v2` 作为“连接管理库”边界，不把它扩展成 SQL 执行代理
- 优先补测试，再改实现
- 没有验证证据，不要宣称“已完成”

## 提交流程

1. 新建分支
2. 先补测试或复现用例
3. 实现最小修复或最小功能
4. 更新 README / 文档
5. 跑完整验证
6. 提交 PR

## 本地验证

根模块：

```bash
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
docker run --rm -v "$PWD":/app -w /app golangci/golangci-lint:v2.11.4 golangci-lint run --config .golangci.yml ./...
```

`v2` 模块：

```bash
cd v2
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
docker run --rm -v "$(pwd)/..":/app -w /app/v2 golangci/golangci-lint:v2.11.4 golangci-lint run --config ../.golangci.yml ./...
```

真实 MySQL 集成测试：

```bash
cd v2
ORM_RUN_INTEGRATION=1 ORM_TEST_DSN='root:root@tcp(127.0.0.1:3306)/' go test -run Integration ./...
```

## 设计约束

- 读写路由相关变更必须覆盖并发与切主路径
- 事务重试相关变更必须覆盖 observer 和回归用例
- 文档如果新增风险提醒，必须写清“库负责什么，不负责什么”
- 新增 Prometheus / OTel 能力时，优先做示例和适配层，不直接把依赖绑进 core

## PR 内容要求

请在 PR 中至少写清：

- 改动目的
- 关键设计取舍
- 风险与兼容性影响
- 验证命令和结果
