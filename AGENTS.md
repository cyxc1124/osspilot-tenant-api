# osspilot-tenant-api

租户 API。对照 OssPilot `legacy/`（v0.6.0）按切片用 Go 重写，不抄 Python。

## 提交

`<type>: <中文说明>`，可加范围：`feat(auth): ...`。必须保留 type 前缀，不要只写中文。

- `feat` 新功能
- `fix` 修缺陷
- `docs` 文档
- `style` 格式（不影响行为）
- `refactor` 重构
- `perf` 性能
- `test` 测试
- `build` 构建与依赖
- `ci` CI / 工作流
- `chore` 脚手架、杂项
- `revert` 回滚

```
feat: 实现租户登录、当前用户和修改密码
fix: 改密后清除 must_change_password
docs: 扩充提交前缀
ci: 增加 healthz 与 Lint/Test/Build
chore: 初始化仓库
```

## 发布

功能 PR 先合进 `develop`。要发版本时：`develop` 开 PR 到 `main`，合并后再打 annotated tag 并 push。不要在 `develop` 上直接打发行 tag。只改 CI Action 不用打 tag。

## 实现

- Go 1.26，`net/http` ServeMux，`pgx/v5`，goose embed。不用 Gin / GORM。启动不做 migrate。
- 一片一变，过 CI 再合。不要顺手做邻片。
- 有新表才加 `migrations/*.sql`。库还没跑过时，改建表脚本，不要补清理迁移。
- 不内置种子用户。
- 暂时不写功能测试；只留能挡住编译的最小检查。
- 错误体 `{"detail":"..."}`。默认端口 `:8000`。
- 前端不打进 API 镜像。
