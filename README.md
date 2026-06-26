# gitea-ai-bot

一个用 Go 编写的 PR 审阅机器人。

它可以：

- 接收 Gitea webhook 常驻运行
- 或以 `oneshot` 方式被命令行/CI 直接调用
- 拉取目标 PR 的上下文与代码工作区
- 调用本机 `opencode` 完成审阅
- 将结果解析为统一结构
- 在支持的平台上回写 review、总结评论和 inline 评论

当前项目优先支持 Gitea；GitHub 目前只支持 `oneshot + dry-run` 的只读审阅。

## 设计目标

这个项目的核心目标不是自己直接接大模型 API，而是做一个稳定的“审阅编排器”：

- 负责平台接入
- 负责工作区准备
- 负责 prompt 组织
- 负责调用 `opencode`
- 负责把结果映射回代码托管平台

这样模型接入、鉴权、工具使用等复杂性都交给 `opencode`，本项目只负责 PR review 的业务闭环。

## 当前能力

### 已支持

- Gitea `oneshot` dry-run
- Gitea `oneshot` wet-run（真实回写）
- Gitea `daemon` 模式入口
- GitHub `oneshot` dry-run
- Profile 驱动的审阅提示词
- 项目内嵌系统提示词（编译进二进制）
- 本地仓库缓存与隔离工作区
- 多行 inline finding 的内部保留
- 统一结构化日志（Go `slog`）

### 目前限制

- GitHub 暂不支持发布 review/comment
- GitHub 暂不支持 daemon/webhook
- Gitea 发布 inline 时，平台侧仍按单点评论落位
  - 但会在评论正文中保留多行范围信息
- 审阅质量仍取决于 `opencode` 当前可用的 provider/model 以及它的工具行为

## 目录结构

```text
cmd/review-bot/             CLI 入口
internal/config/            配置结构与继承解析
internal/core/              核心数据结构
internal/executor/          opencode 执行器与结果读取
internal/loader/            配置/profile 装载器
internal/logging/           slog 日志初始化
internal/platform/          平台分发层
internal/platform/gitea/    Gitea API 适配
internal/platform/github/   GitHub API 适配（当前只读）
internal/profiles/          profile 加载器
internal/publish/           结果发布
internal/resultparser/      opencode 结果解析
internal/review/            审阅主流程
internal/review/prompts/    系统提示词（嵌入二进制）
internal/server/            daemon webhook 入口
internal/triggers/          触发类型与命令解析
internal/workspace/         仓库缓存、隔离工作区、锁
config/profiles/            用户可编辑的 profile 提示词
```

## 审阅流程

无论是 `oneshot` 还是 `daemon`，核心链路都是：

1. 解析参数或 webhook 事件
2. 加载 profile
3. 拉取 PR 元数据与改动文件
4. 准备本地工作区
5. 组装 prompt
6. 调用 `/usr/bin/env opencode`
7. 读取结构化结果文件
8. 输出日志
9. 视模式决定是否回写平台

## Prompt 结构

最终 prompt 由三部分组成：

1. 系统提示词：`internal/review/prompts/system.md`
2. 用户 profile：`config/profiles/<name>/prompt.md`
3. 运行时上下文：PR 标题、描述、改动文件、patch 等

其中系统提示词会通过 `go:embed` 编译进二进制，不依赖运行时文件读取。

## Profile

每个 profile 是一个目录：

```text
config/profiles/default/
  profile.yaml
  prompt.md
```

### `profile.yaml`

当前字段：

- `target`
- `language`
- `inline_enabled`
- `inline_limit`

### `prompt.md`

用于定义：

- 审阅角色
- 关注点
- 输出语言
- `review_action` 判断标准

## 环境变量

程序启动时会自动读取当前工作目录下的 `.env`。

规则：

- 只加载尚未出现在当前环境中的变量
- 不覆盖你显式导出的环境变量
- `.env` 仅用于本项目自己直接读取的平台 token

当前模板变量：

- `GITHUB_TOKEN`
- `GITEA_TOKEN_DEFAULT`
- `GITEA_WEBHOOK_SECRET_DEFAULT`
- `GITEA_TOKEN_BACKEND`
- `GITEA_WEBHOOK_SECRET_BACKEND`

注意：

- `.env` 已加入 `.gitignore`
- `opencode` 自己使用的模型 provider key 不由本项目管理

## 日志

整个项目统一使用 Go 的 `log/slog`。

当前行为：

- 默认输出到终端
- 日志中包含阶段进度
- dry-run / wet-run 都会输出最终 review 结果摘要

常见日志阶段：

- `loading config and profile`
- `fetching pull request context`
- `preparing workspace`
- `running opencode review`
- `review completed`

## 缓存与工作区

默认路径：

- 仓库缓存：`./.cache/repos`
- 隔离工作区：`./.worktrees`

说明：

- 同一仓库会复用 cache repo
- 每次审阅会建立独立 worktree 目录
- 已加入 repo 级文件锁，避免多个进程同时 `git fetch` 竞争 `shallow.lock`

## `oneshot` 模式

`oneshot` 模式不依赖 `config.yaml`，只需要：

- `profiles` 目录
- 命令行参数

### Gitea dry-run 示例

```bash
go run ./cmd/review-bot \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --pr 123 \
  --provider openai \
  --model gpt-5.4 \
  --trigger-type event \
  --event-name pull_request.opened \
  --dry-run \
  --token-env GITEA_TOKEN_DEFAULT
```

### Gitea wet-run 示例

```bash
go run ./cmd/review-bot \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --pr 123 \
  --provider openai \
  --model gpt-5.4 \
  --trigger-type event \
  --event-name pull_request.opened \
  --publish \
  --token-env GITEA_TOKEN_DEFAULT
```

### GitHub dry-run 示例

```bash
go run ./cmd/review-bot \
  --profiles-dir ./config/profiles \
  --platform github \
  --owner nixos \
  --repo nixpkgs \
  --pr 530373 \
  --provider openai \
  --model gpt-5.4 \
  --trigger-type event \
  --event-name pull_request.opened \
  --dry-run
```

说明：

- GitHub 当前只允许 `dry-run`
- 如果 Gitea 是私有实例，通常需要 `--token-env`

## `daemon` 模式

`daemon` 模式会读取 `config.yaml`，当前只支持 Gitea。

示例：

```bash
go run ./cmd/review-bot \
  --daemon \
  --instance corp-gitea \
  --config ./config \
  --listen :8080
```

当前 webhook 入口：

- `POST /webhooks/gitea`

支持的触发来源：

- 自动事件
- PR 评论命令

## 平台差异

### Gitea

- 支持拉取 PR 上下文
- 支持发布 review action
- 支持发布总结评论
- 支持发布 inline 评论

### GitHub

- 当前只支持读取 PR 上下文
- 暂不支持发布结果

## Inline 评论策略

内部结果支持多行范围：

- `start_line`
- `end_line`
- `start_side`
- `end_side`

当前平台处理：

- GitHub：后续可直接映射到多行 review comment
- Gitea：会退化到起始行落点，并在正文里加入 `Lines x-y` 范围标记

## 开发与测试

运行全部测试：

```bash
go test ./...
```

本项目当前已经覆盖的重点测试包括：

- 配置加载与继承
- `.env` 自动加载
- profile 加载
- inline 结果解析
- workspace 并发锁
- `opencode` 执行器参数与结果文件处理

## 已知问题

- `opencode` 的输出结构并不总是完全稳定，解析层已经做了部分兼容，但仍可能继续扩展
- 浅克隆 / grafted 历史会影响 merge-base 相关判断
- 某些包的实际构建验证受本机平台限制，只能做静态审阅

## 后续方向

- GitHub 发布能力
- 更完整的 webhook 行为
- 更稳定的 `opencode` 结果协议
- 更细粒度的平台评论位置映射
- 更完善的 README 示例配置与 CI 集成文档
