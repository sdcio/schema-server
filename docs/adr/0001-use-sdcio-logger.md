# Use github.com/sdcio/logger instead of logrus

schema-server will log through `github.com/sdcio/logger` (logr backed by slog), aligned with [data-server](https://github.com/sdcio/data-server). Standalone `schema-server` and the `schema_upload` CLI bootstrap JSON logging with the same level flags (`-d` / `-t` → `VDebug` / `VTrace`) as data-server. When schema-server code runs inside data-server (local schema store: `pkg/store`, `pkg/schema`, etc.), it must not configure its own handler; it uses the logger data-server already set via `SetDefaultLogger` and, where a `context.Context` is available, `FromContext(ctx)`.

Startup will not dump the full marshaled config blob; log the config file path and a short non-sensitive summary (same direction as tightening operational logging). Trace-heavy YANG parsing logs stay, but use `log.V(logger.VTrace).Enabled()` before emitting (data-server style) to avoid cost when trace is off.

Dependabot dependency PRs (e.g. logrus bumps on [#255](https://github.com/sdcio/schema-server/pull/255)) are left to Dependabot after `logrus` is removed from `go.mod`; no special handling of that PR beyond dropping the direct logrus dependency in the migration.

`NewSchema` and `Reload` take `context.Context` so embedded callers (data-server local schema store) attach parse logs to the parent logger. Standalone `main` and `schemac` call `pkg/logbootstrap.Init` (JSON, `-d`/`-t`, `EXTRA_LOG_FILE`); they do not start pprof. Pin `github.com/sdcio/logger v0.0.3` with data-server.

**Considered:** Keep logrus; text logs; `DefaultLogger` only without `ctx` on `NewSchema` (rejected—embedded mode needs explicit context).
