# osspilot-tenant-api

OssPilot 租户 API（Go）。对照 [OssPilot](https://github.com/cyxc1124/OssPilot) `v0.6.0` 按切片重写，不从 Python 拆目录迁过来。

本仓将包含 HTTP API、`migrations/`（goose）、worker。

## 本地

```bash
go test ./...
go run ./cmd/api
```

默认 `:8000`，`GET /healthz`。

## 许可

AGPL-3.0-only
