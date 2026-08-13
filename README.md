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
- `GET|POST /api/buckets`、`GET|PUT|DELETE /api/buckets/{bucket_name}`（有 S3 时建桶会同时 CreateBucket，并写入 `TENANT_CORS_ORIGINS` 默认 CORS；`versioning_enabled` 会同步 RGW；列表与读写只含当前账号已授权的桶）
- `GET|PUT|DELETE /api/buckets/{bucket_name}/policy`
- `GET|PUT|DELETE /api/buckets/{bucket_name}/cors`
- `GET /api/buckets/{bucket_name}/objects`、`GET .../objects/detail`、`POST .../objects/directories`
- `POST /api/uploads/presign|complete`、`POST /api/uploads/multipart/{init,parts,complete,abort}`
- `POST /api/downloads/presign`、`POST /api/downloads/batch`
- `GET /api/versions`、`GET /api/versions/{id}`、`POST .../download`、`POST .../restore`、`DELETE /api/versions/{id}`
- `GET /api/login-branding`（无需登录）、`GET /api/platform-config`
- `PUT|DELETE /internal/accounts/{username}`、`PUT /internal/accounts/{username}/buckets`（运营投影，Bearer `PROJECTION_SECRET`）

无 `S3_ENDPOINT` / `RGW_ACCESS_KEY` / `RGW_SECRET_KEY` 时上传下载返回 503。

登录品牌与 CDN 域名优先读 `platform_settings`，缺省回退环境变量与内置文案。`storage_region` 暂为 `null`（等运营区域投影）。

租户不内置账号。运营投影写入的账号角色为 `tenant_admin`。租户控制台自建的桶会记成本地授权，运营改授权时不会清掉。对象版本是覆盖前复制到 `.versions/` 的归档（在线编辑 T10 才会大量产生；恢复当前对象时也会先归档）。用户带 `must_change_password=true` 时，除改密、`/api/me`、登出外一律 403。新密码至少 8 位且不能与旧密码相同。

契约见 `openapi.yaml`。无 `DATABASE_URL` 时 healthz 仍可用，鉴权接口返回 503。

## 许可

AGPL-3.0-only
