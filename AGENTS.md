# osspilot-tenant-api

租户 API。对照 OssPilot `legacy/`（v0.6.0）按切片用 Go 重写，不抄 Python。

## 提交

Conventional Commits：`<type>: <中文说明>`。必须保留 type 前缀，包括 `ci:`、`chore:`。

```
feat: 实现租户登录、当前用户和修改密码
ci: 增加 healthz 与 Lint/Test/Build
chore: 初始化仓库
```

不要只写中文、丢掉前缀。

## 实现

- Go 1.26，`net/http` ServeMux，`pgx/v5`，goose embed。不用 Gin / GORM。启动不做 migrate。
- 一片一变，过 CI 再合。不要顺手做邻片。
- 有新表才加 `migrations/*.sql`。库还没跑过时，改建表脚本，不要补清理迁移。
- 不内置种子用户。
- 暂时不写功能测试；只留能挡住编译的最小检查。
- 错误体 `{"detail":"..."}`。默认端口 `:8000`。
- 前端不打进 API 镜像。
