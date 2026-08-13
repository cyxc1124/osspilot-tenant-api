# osspilot-tenant-api

OssPilot 租户 API（Go）。对照 [OssPilot](https://github.com/cyxc1124/OssPilot) `v0.6.0` 按切片重写。

## 本地

```bash
# 需要 Postgres 时先迁移
export DATABASE_URL=postgres://osspilot:osspilot@127.0.0.1:5432/osspilot_tenant?sslmode=disable
go run ./cmd/migrate up

go test ./...
go run ./cmd/api
```

- `GET /healthz`
- `POST /api/login`、`POST /api/logout`、`GET /api/me`、`POST /api/password/change`

租户不内置账号。用户带 `must_change_password=true` 时，除改密、`/api/me`、登出外一律 403。新密码至少 8 位且不能与旧密码相同。

契约见 `openapi.yaml`。无 `DATABASE_URL` 时 healthz 仍可用，鉴权接口返回 503。

## 许可

AGPL-3.0-only
