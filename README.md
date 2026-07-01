# gitea-ai-bot

一个用 Go 编写的代码托管机器人，负责把仓库上下文、工作区准备、提示词组织和 `opencode` 执行串起来。

它目前主要面向 Gitea，也提供一部分 GitHub 只读能力。

支持的能力包括：

- `review`：审阅 PR
- `ask`：对 PR 或 issue 提问
- `summarize`：重写 PR 描述正文
- Gitea webhook 常驻运行
- CLI `oneshot` / CI 直接调用

## 快速上手

### 1. 前置条件

- Go 1.24+
- 本机可用的 `opencode`
- 对应平台的 API token
- 如果要跑 Gitea webhook，还需要 webhook secret

注意：

- 本项目不直接管理模型厂商密钥
- `opencode` 自己需要的 provider key 仍由 `opencode` 侧负责

### 2. 配置 `.env`

最小示例：

```env
GITEA_TOKEN_DEFAULT=your-token
GITEA_WEBHOOK_SECRET_DEFAULT=your-webhook-secret
GITHUB_TOKEN=your-github-token
```

程序启动时会读取当前工作目录下的 `.env`：

- 只补充当前环境里不存在的变量
- 不覆盖你手动导出的环境变量

### 3. 准备 profile

默认 profile 目录结构：

```text
config/profiles/default/
  profile.yaml
  prompt.md
  review.md
  ask.md
  summarize.md
```

最常用的是：

- `review.md`：审阅提示词
- `ask.md`：问答提示词
- `summarize.md`：重写 PR 描述提示词

### 4. 最常用命令

审阅一个 PR：

```bash
go run ./cmd/review-bot review \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --pr 123 \
  --provider openai \
  --model gpt-5.4 \
  --timeout-seconds 300 \
  --dry-run \
  --token-env GITEA_TOKEN_DEFAULT
```

对一个 PR 提问：

```bash
go run ./cmd/review-bot ask \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --pr 123 \
  --provider openai \
  --model gpt-5.4 \
  --timeout-seconds 300 \
  --question "为什么这里要拆成两个 service？" \
  --dry-run \
  --token-env GITEA_TOKEN_DEFAULT
```

对一个 issue 提问：

```bash
go run ./cmd/review-bot ask \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --issue 77 \
  --provider openai \
  --model gpt-5.4 \
  --timeout-seconds 300 \
  --question "这个 issue 在说什么？" \
  --dry-run \
  --token-env GITEA_TOKEN_DEFAULT
```

重写 PR 描述正文：

```bash
go run ./cmd/review-bot summarize \
  --profiles-dir ./config/profiles \
  --platform gitea \
  --base-url http://gitea.example.com \
  --owner team \
  --repo service \
  --pr 123 \
  --provider openai \
  --model gpt-5.4 \
  --timeout-seconds 300 \
  --dry-run \
  --token-env GITEA_TOKEN_DEFAULT
```

### 5. 启动 webhook

```bash
go run ./cmd/review-bot daemon \
  --instance corp-gitea \
  --config ./config \
  --listen :8080
```

当前 webhook 入口：

- `POST /webhooks/gitea`

## 平台支持概览

### Gitea

- 支持 CLI dry-run
- 支持 CLI publish
- 支持 webhook
- 支持 PR review 回写
- 支持 issue / PR comment 回写
- 支持更新 PR 描述正文

### GitHub

- 当前只支持 CLI dry-run / 只读上下文获取
- 暂不支持 publish
- 暂不支持 webhook

## 参考手册

## 设计定位

这个项目不是直接接大模型 API，而是一个“代码托管工作流编排器”：

- 平台 API 接入
- 仓库上下文获取
- 本地工作区准备
- 提示词拼装
- 调用 `opencode`
- 把结果映射回平台

这样模型接入、工具使用、provider 鉴权等复杂性都交给 `opencode`，本项目只处理 review / ask / summarize 的业务闭环。

## CLI 命令

当前提供四个子命令：

- `daemon`：启动 Gitea webhook 服务
- `review`：对 PR 做一次审阅
- `ask`：对 PR 或 issue 提问
- `summarize`：重写 PR 描述正文

查看帮助：

```bash
go run ./cmd/review-bot --help
go run ./cmd/review-bot review --help
go run ./cmd/review-bot ask --help
go run ./cmd/review-bot summarize --help
```

### `review`

用途：

- 拉取 PR 元数据和 diff
- 准备对应 revision 的工作区
- 调用 `opencode`
- 解析结构化审阅结果
- 可选回写 review/comment

常用参数：

- `--pr`
- `--profile`
- `--provider`
- `--model`
- `--timeout-seconds`
- `--dry-run`
- `--publish`

### `ask`

用途：

- 对 PR 提问
- 对 issue 提问

规则：

- 必须二选一：`--pr` 或 `--issue`
- 对 PR 提问时，基于 PR 标题、正文、diff 和对应代码工作区回答
- 对 issue 提问时，基于 issue 标题、正文和仓库默认分支 HEAD 的工作区回答

常用参数：

- `--pr` 或 `--issue`
- `--question`
- `--profile`
- `--provider`
- `--model`
- `--timeout-seconds`
- `--dry-run`
- `--publish`

### `summarize`

用途：

- 生成新的 PR 描述正文
- 可选直接更新 PR body

`--dry-run`：

- 只输出建议正文

`--publish`：

- 保留原 PR body
- 在原正文后追加：

```md
---

<!-- review-bot:summarized source=<source> -->

<总结正文>
```

幂等规则：

- 如果 PR body 中已存在上述 marker
- 再次运行时会直接退出

## Webhook 命令

当前支持的评论命令：

- `/review`
- `/review <profile>`
- `/ask <问题>`
- `/ask <profile> <问题>`
- `/summarize`
- `/summarize <profile>`

### `/ask` 在 issue 上的行为

普通 issue comment 上的 `/ask` 会：

- 读取 issue 标题和正文
- 解析仓库默认分支
- 使用默认分支 HEAD 准备工作区
- 让 `opencode` 基于该上下文回答问题

### `/summarize` 的权限规则

只针对 Gitea webhook 生效。

1. 如果评论人是 PR 作者
    - 直接允许更新 PR 描述
2. 如果评论人不是 PR 作者
    - 查询该用户在仓库中的权限
        - `write/push` 或 `admin`：允许更新 PR 描述
        - 否则：不更新 PR 描述，转为发一条普通评论

无权限时的评论格式：

```md
@<user> 你当前没有修改这个 PR 描述的权限，下面是可手动采用的建议正文。

---

<总结正文>
```

如果 PR 已经总结过：

- 直接按 marker 判断
- 不再重复更新
- 会提示该 PR 已由谁总结过

## 配置文件

默认配置文件：`config/config.yaml`

当前主要结构：

```yaml
models:
  default-review:
    provider: openai
    model: gpt-4.1-mini
  default-ask:
    provider: openai
    model: gpt-5.4

instances:
  github-public:
    platform: github
    base_url: https://api.github.com
    config:
      review_model: default-review
      ask_model: default-ask
      opencode_timeout_seconds: 300
      command_review_enabled: true
      allowed_commands: [review, ask, summarize]
      default_profile: default
      enabled_profiles: [default]
```

重要字段：

- `review_model`
- `ask_model`
- `opencode_timeout_seconds`
- `command_review_enabled`
- `allowed_commands`
- `default_profile`
- `enabled_profiles`

### 超时

统一超时字段：

- `opencode_timeout_seconds`

作用范围：

- `review`
- `ask`
- `summarize`

CLI 可用 `--timeout-seconds` 覆盖。

## Profile 结构

每个 profile 是一个目录：

```text
config/profiles/default/
  profile.yaml
  prompt.md
  review.md
  ask.md
  summarize.md
```

### `profile.yaml`

当前字段：

- `target`
- `language`
- `inline_enabled`
- `inline_limit`

### 提示词文件

- `prompt.md`
  - 通用回退提示词
  - 当专属提示词缺失时使用
- `review.md`
  - `review` 专属提示词
- `ask.md`
  - `ask` 专属提示词
- `summarize.md`
  - `summarize` 专属提示词

推荐为不同能力分别维护独立文件，不要把审阅、问答、正文改写混在一个文件里。

### 固定系统提示词

用户可编辑的是 `config/profiles/*/*.md`。

程序内固定系统提示词来自 embedded markdown：

- `internal/review/prompts/`
- `internal/ask/prompts/`
- `internal/summarize/prompts/`

这些文件会在构建时打进二进制。

## 环境变量

程序启动时会自动读取当前工作目录下的 `.env`。

当前常用变量：

- `GITHUB_TOKEN`
- `GITEA_TOKEN_DEFAULT`
- `GITEA_WEBHOOK_SECRET_DEFAULT`

注意：

- `.env` 只用于本项目自己直接读取的平台 token / secret
- `opencode` 需要的 provider 凭据不由本项目管理

## 缓存与工作区

默认路径：

- 仓库缓存：`./.cache/repos`
- 隔离工作区：`./.worktrees`

行为：

- 同一仓库会复用 cache repo
- 每次执行会建立独立工作区目录
- 有 repo 级锁，避免并发 `git fetch` 冲突

## 日志

整个项目统一使用 Go 的 `log/slog`。

常见阶段日志：

- `loading config and profile`
- `fetching pull request context`
- `fetching ask context`
- `preparing workspace`
- `running opencode review`
- `running opencode ask`
- `running opencode summarize`

## 项目结构

```text
cmd/review-bot/             CLI 入口
internal/ask/               问答主流程
internal/config/            配置解析与继承
internal/core/              核心数据结构
internal/executor/          opencode 执行器
internal/loader/            配置/profile 装载
internal/platform/          平台分发层
internal/platform/gitea/    Gitea API 适配
internal/platform/github/   GitHub API 适配（当前只读）
internal/profiles/          profile 加载器
internal/review/            审阅主流程
internal/server/            webhook 入口
internal/summarize/         PR 描述重写主流程
internal/triggers/          webhook 命令解析
internal/workspace/         仓库缓存与工作区管理
config/profiles/            用户可编辑 profile
```

## 平台限制与已知边界

- GitHub 当前只支持只读上下文获取，不支持 publish
- GitHub 当前不支持 webhook
- Gitea inline 评论仍按单点落位，但正文会保留多行范围信息
- `opencode` 输出仍可能随版本变化，需要解析层继续兼容
- 浅克隆 / grafted 历史可能影响某些比较逻辑

## 开发与测试

运行全部测试：

```bash
go test ./...
```

当前重点测试覆盖：

- 配置加载与继承
- profile 加载与能力专属提示词
- webhook 命令解析
- issue ask / PR ask / summarize 主流程
- Gitea 平台回写与权限判断
- `opencode` 执行器与超时处理
- workspace 缓存与并发锁

## 后续方向

- GitHub publish 支持
- 更完整的 Gitea / GitHub 权限模型接入
- 更稳定的 `opencode` 结果协议
- 更丰富的 profile 示例
