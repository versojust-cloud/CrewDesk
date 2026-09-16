# Dream-Waver

> 一个开源的 Multi-Agent 平台 — Manus / Genspark 类产品的 Go + Rust 实现。
> MVP 以 **AI PPT 生成** 为首个能力，长期目标是通用 Agent 平台 (Slides / Sheets / Docs / Code / Video)。

## 设计目标

- **Multi-Agent 架构**：Planner → Researcher → Worker，配合 ReAct 工具调用循环
- **生产可上线**：SaaS 化（多租户、Credit 计费、可观测、可扩展）
- **性能 & 安全**：Go orchestrator 负责并发编排，Rust 沙箱负责隔离执行用户/Agent 代码
- **HTML-first PPT**：Tailwind 模板 + chromedp 渲染 + unioffice 组装 PPTX；用户拿到的是可编辑的真 PPTX
- **多 LLM 可切换（尚未落地）**：目前只接入了 DeepSeek v4-pro（OpenAI 兼容 API）。
  多 provider 路由还在路线图 Week 4 —— `config.go` 的 provider 分支现在只实现 deepseek，
  把 `LLM_PRIMARY_PROVIDER` 切成 anthropic / openai 会直接启动失败

## 仓库结构

```
Dockerfile            # orchestrator 镜像（构建上下文是仓库根，见文件头注释）
Makefile              # 常用命令：make dev / orchestrator / web / test / proto …
docker-compose.yml    # 本地开发全栈（postgres / redis / minio / sandbox / orchestrator / web）
fly.toml              # Fly.io 部署配置（orchestrator）
services/
  orchestrator/       # Go：主服务（API、Agent、LLM、PPT 工具）
  sandbox/            # Rust：gRPC 沙箱（wasmtime 隔离执行代码工具）
  dreamapi-sidecar/   # Python：dreamapi 能力边车
apps/
  web/                # Next.js 15：用户界面
packages/
  slide-templates/    # HTML+Tailwind 模板库
proto/                # 共享 gRPC schema（含生成的 Go / Rust 绑定）
scripts/              # 辅助脚本（Supabase 初始化）
output/               # 各 skill 的生成产物，只有 README 入库
docs/                 # 架构与开发文档
.github/workflows/    # CI
```

## Quickstart

```bash
# 1. 复制环境变量
cp .env.example .env
# 编辑 .env，填 DEEPSEEK_API_KEY —— 这个是必填：默认 provider 就是 deepseek，
# 缺 key 时 orchestrator 会在启动校验阶段直接退出。目前也只有这一个 provider 可用。

# 2. 启动开发环境
make dev    # 等同 docker-compose up --build

# 3. 打开浏览器
open http://localhost:3000
```

依赖（本地裸跑而非 docker）：
- Go 1.26+（以 `services/orchestrator/go.mod` 为准）
- Rust 1.80+
- Node 20+ 与 pnpm 10+（前端锁文件是 pnpm 10 格式）
- Postgres 16、Redis 7、MinIO（或用 docker-compose 起）
- protoc + protoc-gen-go + protoc-gen-go-grpc + tonic（用于生成 gRPC 代码）

## 路线图

详见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) 与 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。

- **Week 1**：仓库骨架 + Agent 抽象 + Rust 沙箱 hello
- **Week 2**：PPT 端到端跑通（背景图模式）
- **Week 3**：双层 PPTX（可编辑文本）+ 模板扩到 5 套
- **Week 4**：联网研究 + 多模型路由 + Claude prompt caching
- **Week 5**：SaaS 化（Stripe + Credit-wallet + 用量监控）
- **Week 6**：上线 Fly.io / Vercel + Beta 邀请

## 借鉴的开源项目

仅借鉴机制，未 fork 代码：
- [OpenManus](https://github.com/FoundationAgents/OpenManus) — Agent 三层抽象 (Base → ReAct → ToolCall)
- [Presenton](https://github.com/presenton/presenton) — HTML+Tailwind → PPTX 思路
- [PPTAgent](https://github.com/icip-cas/PPTAgent) — 反思式生成 + PPTEval 评估

## License

MIT
