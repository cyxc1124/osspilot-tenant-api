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

Worker（扫描 S3 回填 `object_records`，并按 `platform_settings` 清过期回收站）不跑迁移：

```bash
export REDIS_URL=redis://127.0.0.1:6379/0
go run ./cmd/worker
```

未设 `REDIS_URL` 时 worker 退出。API 不依赖 Redis。

- `GET /healthz`
- `POST /api/login`、`POST /api/logout`、`GET /api/me`、`POST /api/password/change`
- `GET|POST /api/buckets`、`GET|PUT|DELETE /api/buckets/{bucket_name}`（有 S3 时建桶会同时 CreateBucket，并写入 `TENANT_CORS_ORIGINS` 默认 CORS；`versioning_enabled` 会同步 RGW；列表与读写只含当前账号已授权的桶）
- `GET|PUT|DELETE /api/buckets/{bucket_name}/policy`
- `GET|PUT|DELETE /api/buckets/{bucket_name}/cors`
- `GET /api/buckets/{bucket_name}/objects`、`GET .../objects/detail`、`POST .../objects/directories`、`DELETE .../objects`（默认搬进 `.trash/`；`permanent=true` 直接删）
- `GET|DELETE /api/buckets/{bucket_name}/trash`、`POST .../trash/restore`（列表/清空/恢复；列表读 `object_records`，恢复与清空走 RGW）
- `POST /api/uploads/presign|complete`、`POST /api/uploads/multipart/{init,parts,complete,abort}`
- `POST /api/downloads/presign`、`POST /api/downloads/batch`
- `GET /api/preview/{text,image,video,audio,pdf}`（文本限 512KiB；媒体走预签名 GET，暂不改写 CDN）
- `GET /api/versions`、`GET /api/versions/{id}`、`POST .../download`、`POST .../restore`、`DELETE /api/versions/{id}`
- `GET|POST /api/share-links`、`DELETE /api/share-links/{id}`、公开 `GET /s/{token}`（可选 `?password=`，返回预签名 URL）
- `GET /api/file-locks/status`、`POST /api/file-locks/{lock,refresh,unlock}`
- `POST /api/text-edit/open`、`POST /api/text-edit/{session_id}/save`、`POST /api/text-edit/{unlock,close}`
- `POST /api/editor/open`、`POST /api/editor/save`、`POST /api/editor/unlock`；ONLYOFFICE 拉文件 `GET /api/editor/files/{id}/download`、回调 `POST /api/editor/callback/{id}`
- `GET /api/login-branding`（无需登录）、`GET /api/platform-config`
- `GET /api/roles`、`GET|POST /api/users`、`GET|PUT|DELETE /api/users/{id}`
- `GET|POST /api/user-groups`、组成员增删
- `GET|POST /api/permissions`、`PUT|DELETE /api/permissions/{id}`
- `GET|POST /api/permission-templates` 及 rules / assignments
- `GET /api/api-access`、`POST /api/api-access/request`（开通状态；批准走运营 `POST /internal/api-access/{username}/approve`）
- `PUT|DELETE /internal/accounts/{username}`、`PUT /internal/accounts/{username}/buckets`、`GET /internal/api-access`、`GET|POST /internal/api-access/{username}/...`（运营投影，Bearer `PROJECTION_SECRET`）
- `GET|POST /api/applications`、`PUT|DELETE /api/applications/{id}`、密钥增删；创建密钥只此一次返回 `secret_access_key`
- `POST /api/sts/credentials`（本仓 JWT 作 session_token，不是 RGW STS）
- `GET /api/v1/buckets`、`GET /api/v1/buckets/{name}`、`GET .../objects`、`POST /api/v1/uploads/presign`、`POST /api/v1/downloads/presign`、`POST /api/v1/sts/credentials`（`Authorization: OSSAccessKey id:secret` 或 `OSSSession id:secret:token`）
- `GET /api/stats`、`GET /api/stats/buckets`（从 `object_records` 聚合 live / `.trash/` / `.versions/`；账号配额暂无）
- `GET /api/stats/traffic`、`GET /api/stats/buckets/requests`（无访问日志时全 0）
- `GET /api/audit-logs`、`GET /api/audit-logs/export`（CSV 中文表头；`tenant_admin` / `audit_user` / `audit` 动作）
- `GET /api/alerts/notifications`（只读 `alert_events`，规则引擎在运营侧）

无 `S3_ENDPOINT` / `RGW_ACCESS_KEY` / `RGW_SECRET_KEY` 时上传下载返回 503。

登录品牌与 CDN 域名优先读 `platform_settings`，缺省回退环境变量与内置文案。`storage_region` 暂为 `null`（等运营区域投影）。

租户不内置账号。运营投影写入的账号角色为 `tenant_admin`，`account_id` 等于自身 id。控制台可在账号下建子用户 / 组 / 权限规则 / 模板；`tenant_admin` 仍可做现有全部操作，其他角色按最长前缀规则鉴权（`admin` 覆盖其余动作），看不见的桶仍 404。租户控制台自建的桶会记成本地授权，运营改授权时不会清掉。对象版本是覆盖前复制到 `.versions/` 的归档（文本编辑保存时会先归档；恢复当前对象时也会先归档）。删除对象默认复制到 `.trash/{原 key}` 并改写 `object_records`（不是单独的回收站表）；回收站列表剥掉 `.trash/` 前缀。大批量删除目前同步执行。`cmd/worker` 用 asynq + Redis 定时扫桶回填清单（补 T3），并在 `trash_cleanup_enabled` 时按 `trash_retention_days` 清 `.trash/`。分享链接存在 `share_links`；缺省 7 天过期，访问时签发短时预签名 GET（上限与下载预签名相同）。预签名 URL 暂不改写 CDN（与 T4 下载一致）。在线编辑用 `file_locks`（TTL 2h）和 `edit_sessions`（TTL 8h）；别人占用时以只读打开。ONLYOFFICE 需要 `OFFICE_URL`（也可写在 `platform_settings.office_url`），以及 Document Server 能访问到的 `TENANT_API_PUBLIC_URL`。用户带 `must_change_password=true` 时，除改密、`/api/me`、登出外一律 403。新密码至少 8 位且不能与旧密码相同。应用密钥用 bcrypt 存 hash；STS 签发本仓 JWT（时长默认 3600 秒，最短 900，最长 12 小时），等 O11 再接真 RGW STS。对象删、上传完成、分享、用户管理、凭证会写 `audit_logs`。桶 `used_bytes` / `object_count` 从 `object_records` 聚合。

契约见 `openapi.yaml`。无 `DATABASE_URL` 时 healthz 仍可用，鉴权接口返回 503。

## 许可

AGPL-3.0-only
