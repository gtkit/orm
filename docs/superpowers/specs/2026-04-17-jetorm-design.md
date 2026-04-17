# go-jet Infrastructure Layer Design

## Goal

Add a MySQL-focused `go-jet` infrastructure package to this repository without turning the repository into a second ORM. The package should own connection lifecycle, safe defaults, transaction helpers, and execution helpers while leaving query composition to `go-jet` itself.

## Scope

In scope:

- A new `jetorm` package under the root module.
- MySQL connection configuration with safe defaults.
- `Client` abstraction over `*sql.DB`.
- Transaction helper with automatic commit / rollback handling.
- Thin execution helpers for `go-jet/mysql.Statement`.
- Documentation for package usage and generator workflow.

Out of scope:

- Repository abstraction.
- CRUD scaffolding.
- Generated model code committed into this repository.
- Cross-database abstraction.

## Design

### Package Boundary

`jetorm` is a sibling package to the existing GORM-based APIs. It does not share global mutable configuration with `orm` v1 or `orm/v2`.

### Connection Model

`jetorm.Open(ctx, opts...)` constructs a `Config`, opens a `*sql.DB`, applies pool settings, verifies connectivity with `PingContext`, and returns a `*Client`.

`jetorm.OpenWithDB(db, cfg)` wraps an existing `*sql.DB` without taking ownership of its lifecycle.

### Client API

The first version of `Client` exposes:

- `DB() *sql.DB`
- `Config() Config`
- `PingContext(ctx)`
- `Stats()`
- `Close()`
- `ExecContext(ctx, stmt mysql.Statement)`
- `QueryContext(ctx, stmt mysql.Statement, dest any)`
- `Rows(ctx, stmt mysql.Statement)`
- `WithTx(ctx, opts, fn)`

The package stays intentionally small. Callers still build SQL directly with `go-jet`.

### Context and Timeout Policy

Execution helpers normalize nil contexts to `context.Background()`. If `Config.QueryTimeout` is set and the incoming context has no deadline, the helper derives a timeout context for the operation.

### Transaction Policy

`WithTx` starts a `sql.Tx`, passes a lightweight `Tx` wrapper into the callback, rolls back on returned error or panic, and commits on success. A nil callback is rejected.

The transaction wrapper exposes the same execution helpers as `Client`, but backed by `*sql.Tx`.

### Logging

The package does not set Jet's package-global logger automatically in v1 of the design. That avoids cross-package global side effects. Logging remains an integration concern at the application boundary for now.

### Generator Workflow

The repository documents a supported layout:

- generated query code lives under a consumer-owned path such as `internal/dbgen`
- generation is performed with `jet` CLI outside this library package
- this repository provides conventions and examples, not generated artifacts

## Files

- `jetorm/config.go`: config, defaults, DSN helpers, options.
- `jetorm/client.go`: client lifecycle and execution helpers.
- `jetorm/tx.go`: transaction wrapper and `WithTx`.
- `jetorm/client_test.go`: TDD coverage for lifecycle, timeout, and transaction behavior.
- `README.md`: new `go-jet` section.
- `CHANGELOG.md`: unreleased notes.

## Release Impact

This is a backward-compatible additive change to the root module and should ship as the next `v1` patch release.
