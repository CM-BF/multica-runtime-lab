# AH-13 / DA-1：Deep Agents ACP 最小接入核验

结论：**PASS（架构交付）**。推荐新增 `deepagents` Backend，启动官方 `dcode --acp`，复用已有 ACP transport。Deep Agents 拥有模型循环、工具和内部子 agent；Multica 保留任务调度、上下文准备、事件归一化和进程生命周期。本报告不是实现或运行验收；真实安装、握手、模型 E2E 和恢复测试均未执行，必须在 DA-2/DA-3 按下列契约补齐。

范围仅 DA-1；没有启动 OpenAI 阶段，没有修改代码、数据库、服务、CLI、daemon 或已有 runtime 配置，没有读取账户令牌。

## 1. 基线及可复查证据

核验日期：2026-09-10。fork 基线 `cf946dc9a67811619ebdba100ec0f4055ef7778e`；报告分支 `docs/ah-13-da-1-deepagents-acp`。下述本地代码行号均以此基线为准。

| 检查/命令 | 结果与退出码 |
| --- | --- |
| `git remote -v`，在指定 checkout 执行 | 0；fetch/push 均为 `https://github.com/CM-BF/multica-runtime-lab.git` |
| `git status --short --branch` | 0；初始 `main...origin/main`，无改动 |
| `cat AGENTS.md CLAUDE.md` | 0；已读：根 AGENTS 指向 CLAUDE；遵循包边界、测试不得运行环境中的 agent CLI、真实测试双门禁及数据库隔离规则 |
| `git rev-parse HEAD` | 0；上述 fork SHA |
| `git ls-remote https://github.com/langchain-ai/deepagents.git HEAD` | 0；`481773caed336c86034e1c72b4988e7e93968610` |
| `curl -fsSL https://raw.githubusercontent.com/langchain-ai/deepagents/main/libs/acp/pyproject.toml` | 0；库版本 `0.0.11`，Python `>=3.11`，依赖 `agent-client-protocol>=0.10.1`；随后使用固定 SHA 读取源码 |
| Python `urllib.request` 读取 PyPI `/pypi/deepagents-code/json` 与 `/pypi/deepagents-acp/json`，下载 wheel 到内存，以 `zipfile.ZipFile` 读取下列源文件并计算 SHA256 | 0；发布版本分别为 `0.1.68`、`0.0.11`；dcode 要求 Python `>=3.12,<4.0`；**没有安装或执行 wheel** |
| `command -v uv` / `python3 --version` | 0；uv 可用；Python `3.13.3`；未改全局工具 |
| 固定 SHA 下 `main.py`、`server.py`、`sessions.py`、`agent.py`、`mcp_tools.py` 的公开 HTTPS 只读查询 | 0；关键发布 wheel 行号/逻辑与本报告引用一致 |
| `git switch -c docs/ah-13-da-1-deepagents-acp` | 0；只为报告建立工作分支 |

wheel 定位：

- `deepagents_acp-0.0.11-py3-none-any.whl`，SHA256 `ca91d70d27fbeb9be48ca18e5ce206752a37f2c31dba0b44e0ff55802f063593`。
- `deepagents_code-0.1.68-py3-none-any.whl`，SHA256 `050462a6a6cef17ffb4067c604e35cfc8615116d351312f9aea4fac1650483e2`。
- [PyPI ACP 元数据](https://pypi.org/pypi/deepagents-acp/0.0.11/json)、[PyPI Code 元数据](https://pypi.org/pypi/deepagents-code/0.1.68/json)。固定顶层版本不是完整依赖锁；实施者须保存解析后的完整版本清单。

检索纠错记录：初次猜测 `server/pkg/agent/acp.go`、`registry.go`、`server/pkg/runtime*` 等路径不存在，`rg` 报错或 zsh glob 无匹配；这些是未命中的探索，不是验证成功。随后通过 `rg --files` 定位到 `hermes.go`、`acp_session.go`、`builtin_runtimes.go` 等实际文件。没有重试外部执行失败，没有启动任何服务。代码测试本阶段未执行，因为只交付设计。

## 2. 官方入口及方案取舍

官方 [ACP README（固定提交）](https://github.com/langchain-ai/deepagents/blob/481773caed336c86034e1c72b4988e7e93968610/libs/acp/README.md) 提供预置 coding agent 的 `dcode --acp` 和自定义 `AgentServerACP` 两条路径。首版选择前者：不引入自写 Python agent factory，不复制 agent loop。建议安装命令为 `uv tool install 'deepagents-code==0.1.68' --with 'deepagents-acp==0.0.11'`；这是**供独立开发环境使用的待执行说明**，本阶段未运行，也不允许用它更新正在服务的环境。

生产实现启动形状：`<resolved-dcode> <profile-fixed-args> --acp --model <provider:model> --mcp-config <task-private-json>`。空 model 不传；无 MCP 配置时不生成该参数。进程 cwd 与 ACP `cwd` 必须是同一绝对任务目录。环境从已有授权配置注入，命令日志不能输出秘密。禁用冲突的 `--acp`、交互/一次性启动模式、`--resume` 以及重复 model/MCP 控制参数；校验 fixed/extra/custom 三层 argv，不能只过滤 custom。

另外两种选择：

1. 把 `dcode` 伪装成 Hermes/Kimi profile：改动少但不可用。已有 Backend 硬编码 `acp` 子命令、恢复/模型策略和运行时特例；dcode 使用 `--acp`，恢复为 `session/load`。
2. 自写 `AgentServerACP(create_deep_agent(...))` bridge：能控制 MCP 与持久化，但须维护 agent 构造、工具、存储，薄适配成本更高，首版不选。`python -m deepagents_acp` 的 `__main__.py` 启动 `_serve_test_agent`，不能作为正式 coding runtime 入口。

## 3. 两个必须显式处理的上游差异

### MCP：配置文件接入，不能只发送 session/new

[dcode main.py](https://github.com/langchain-ai/deepagents/blob/481773caed336c86034e1c72b4988e7e93968610/libs/code/deepagents_code/main.py#L3544) 在启动时调用 `resolve_and_load_mcp_tools(explicit_config_path=...)`，随后把工具列表传入 `create_cli_agent`；当前 `build_agent(context)` 不读取 `context.mcp_servers`。因此即使基础 ACP 库保存 `mcpServers`，也不能证明 dcode 将其变成工具。

首版由 Backend 将 `ExecOptions.McpConfig` 校验后写入权限 0600 的任务私有临时 JSON，通过 `--mcp-config` 交付；新建与恢复都重新准备文件。ACP `session/new/load` 传 `mcpServers: []`，避免虚假声称走 ACP 注入或双重加载。文件保留到进程和 MCP 子进程退出后再清理，日志只记是否配置、数量及脱敏错误。

[mcp_tools.py:3284](https://github.com/langchain-ai/deepagents/blob/481773caed336c86034e1c72b4988e7e93968610/libs/code/deepagents_code/mcp_tools.py#L3284) 证明 explicit 文件加入加载列表，但它不是隔离其他配置的开关：用户/项目/插件配置仍可能参与。实施不得添加全项目信任旗标；独立 smoke 使用新 profile、空工作目录及明确测试 MCP 文件。正常运行保留 dcode 自身信任规则，Multica 不再从全局目录重复导入配置。`--no-mcp` 与已配置 agent MCP 冲突应在启动前拒绝。

### 恢复：load、存储、回放必须同时成立

[ACP server.py:346](https://github.com/langchain-ai/deepagents/blob/481773caed336c86034e1c72b4988e7e93968610/libs/acp/deepagents_acp/server.py#L346) 声明 `loadSession` 取决于构造参数；`load_session` 核验 checkpoint 元数据和原 cwd，回放后返回；`cancel` 仅设取消标志，流循环再次推进时才观察。dcode 在 `main.py:3579–3642` 使用 checkpointer 并设置 `load_sessions=True`；[sessions.py:1558](https://github.com/langchain-ai/deepagents/blob/481773caed336c86034e1c72b4988e7e93968610/libs/code/deepagents_code/sessions.py#L1558) 使用 SQLite checkpointer。源码支持不等于已通过跨进程测试。

首版 pin `DEEPAGENTS_HOME` 到 runtime/agent/conversation 专用的持久状态根。`sessions.py:344–363` 使用 `DEFAULT_STATE_DIR / "sessions.db"`，`model_config.py:771` 将该目录绑定为 `PATHS.profile.state_dir`，即 profile 内 `.state/sessions.db`。不要设置臆测的数据库覆盖变量：辅助检查脚本出现的 `DEEPAGENTS_SESSIONS_DB` 不是此运行入口的存储契约。目录须跨同一对话的进程保留；不同工作区/agent/profile 不共用。不得复制已有全局 profile、凭据或数据库。其他文件记忆/产物与 checkpoint 是不同状态，不能把清理临时目录等同于清理对话。实施前对照 `_paths.py` 和 `sessions.py` 的实际环境解析，为这一布置补测试。

## 4. Backend 契约与协议状态机

无需修改 `Backend.Execute(ctx,prompt,ExecOptions) (*Session,error)` 公共接口。现有 `server/pkg/agent/agent.go:21,178,233` 已容纳消息流、终态、会话 ID 和恢复拒绝。复用 `hermesClient`（`hermes.go:900` 起）的 JSON-RPC 请求关联、写锁、读取/通知解析与工具事件处理；复用 `acpDeliverableTracker`、进程树回收和适用的错误分类 helper。不要复制整个 Hermes Backend，也不要把 Hermes provider error/usage 扫描特例搬给 Deep Agents。若确需 transport 小扩展，仅增加新 Backend 所需行为并保持既有 runtime 回归覆盖。

顺序：构造并校验 argv/环境/MCP → 启动受控 stdio 子进程 → `initialize(protocolVersion:1, clientCapabilities:{})` → 检查版本与能力 → `session/new` 或 `session/load` → 配置模型 → 发布 running/SessionID → 发送一次 `session/prompt` → 流式归一化 → 判定终态 → 关闭 stdin、限时排空并回收 owned 进程树 → 关闭 Messages → Result 恰好发送一次并关闭。

模型首选启动参数 `--model provider:model`；恢复后读取 `configOptions`，若 opts 指定的模型与恢复值不同，按返回的 model selector ID 调用 `session/set_config_option`。不得套用 `session/set_model`。发现阶段复用 `discoverACPModels` 的 configOptions 解析，但用新启动参数；失败时允许手填 model，不能伪造已验证模型目录。不支持 reasoning/service tier/MaxTurns 的显式设置须拒绝或在 API 隐藏，不静默忽略。

仅在 capability 宣告时调用可选方法。当前适配只实现 `session/load`，不调用 `session/resume`；load 返回不含 sessionId 时沿用请求 ID。历史回放不得进入当前 Output、工具计数或用量；回放阶段仍保持 RPC/权限响应通畅。状态 pin 仅在 setup 成功且 prompt 即将发出时出现，setup 失败不能发布新建空会话指针。

取消应先发送无 id 的 `session/cancel` 通知，使用独立短清理 deadline 等待 prompt 结束，再回收本次拥有的进程树；不能让 Execute 的 ctx 取消立即销毁管道而根本未送达 cancel。SDK 可能在长工具中不及时观察标志，故硬回收是必需兜底。进程等待与 stdout/stderr 排空都必须有界并 join，不能在 Result 后留下 run-owned 工作。

| 观察 | Multica Result | 会话/重试语义 |
| --- | --- | --- |
| 有效 prompt 响应 `end_turn`，无 RPC/协议错误 | completed | 返回当前 ACP ID；空正文可保留，但不能仅因进程退出 0 判成功 |
| stopReason `cancelled` 或用户取消 | aborted | 不设 ResumeRejected，不自动重跑 |
| 执行/握手 deadline | timeout | 不因超时丢弃原指针；未知副作用不得自动重放 |
| `max_tokens` / `max_turn_requests` / `refusal` / 未知或缺少 stopReason | failed，保留原因及部分输出 | 保守处理，不能当正常完成 |
| EOF、非 JSON stdout、RPC error、无 prompt 响应的退出 0/非 0 | failed | 保留可恢复指针；脱敏诊断；不从任意模型文本猜终态 |
| load 明确找不到指定会话 | failed + ResumeRejected=true | 仅 setup 边界可允许既有 fresh fallback；带连续性提示 |
| load cwd 不一致、DB 不可用、auth/MCP/network 错误 | failed，ResumeRejected=false | 不能通过清空指针掩盖配置/基础设施错误 |
| 请求恢复但 loadSession=false/缺失 | failed，ResumeRejected=false | 明确版本/能力不支持，不自动新建；避免降级后重复执行 |

对 `resource_not_found` 的识别要用实际 ACP Python SDK 序列化 fixture 验证。`acp_session.go` 当前文字匹配未必识别 Deep Agents 返回格式；如需调整，应限定为 load RPC 的明确资源缺失证据，不能把所有 -32602/-32000 当会话丢失。还要审查 daemon `shouldRetryWithFreshSession`，保证新 provider 不走“无法检测恢复拒绝”的宽泛回退。

协议依据：[ACP v1 initialization](https://agentclientprotocol.com/protocol/v1/initialization)、[session setup](https://agentclientprotocol.com/protocol/v1/session-setup)、[prompt turn](https://agentclientprotocol.com/protocol/v1/prompt-turn)。以上状态映射是 Multica 首版设计决策，不是声称上游会产生全部 stop reason。

## 5. 能力及恢复语义矩阵

| 能力 | 已核验证据 | 首版契约 / 尚需验证 |
| --- | --- | --- |
| 文本输入与流输出 | ACP prompt/update；已有 hermesClient 处理 agent_message_chunk | 支持；SDK/fake 两层验证 chunk 顺序及最终正文 |
| 工具事件、内部 subagent | SDK astream 使用 subgraphs；已有 tool_call/update 处理 | SDK 拥有工具执行；Multica 按 call ID 归一化，不能以标题当唯一 ID；验证失败工具和嵌套事件不挂 watchdog |
| 上下文 | dcode `agent.py:2927` 将项目 AGENTS 路径加入 MemoryMiddleware | execenv 输出 AGENTS.md；`SystemPrompt` 默认常为空，不能只依赖它；DA-3 做 canary 证明实际读取 |
| MCP stdio | dcode explicit config 工具加载路径 | 用私有配置文件；真实本地 MCP 列表/调用契约必须通过 |
| MCP HTTP/SSE | 基础 initialize 未声明 remote MCP capability | 不经 ACP 发送 remote entry；CLI 文件路径上的每种 transport 单独测，未测标未验证，不宣称都支持 |
| 产物 | 本地文件 backend，以 session cwd 构建 | 原地工作目录往返，沿用 Multica 附件交付；文件落盘不等于已上传；测试生成文件并校验内容/附件映射 |
| 模型选择 | dcode model 参数与 ACP configOptions | 支持初始指定；恢复需重应用选择；无模型目录时手填 |
| 图片/音频/资源 | SDK 宣告 image；现有 Execute 只有 string prompt | 首版文本；不在 UI 宣称多模态支持 |
| 取消 | SDK 设置 flag，流循环检查 | 协作取消 + 有界进程树回收，长工具取消仍需真实验证 |
| 跨进程恢复 | load_sessions=True、SQLite、cwd 检查 | 同状态根+同 cwd+原 ID；load 回放不计当前产出；杀进程后的 exactly-once 工具执行**不保证** |
| 终态/用量 | SDK prompt end_turn/cancelled；已有 Result/usage helpers | 未提供用量即未知，不读取其他 runtime 会话文件、不伪造零成本 |
| 权限交互 | hermesClient 有 session/request_permission 处理 | 明确沿用当前无人值守授权策略；仅选择 agent 实际提供的许可选项；不支持的自由输入请求报错，不自行创造答案 |
| Skills | execenv 有 provider-specific skills 路径 | 按 dcode 项目目录规则接入 `.deepagents/skills` 并测，或明确禁用该入口；不能把 staged 文件存在当作 SDK 已加载 |

## 6. 改动清单和串行文件所有权建议

这是给 leader 的分配建议，**不是启动阶段或委派**。

| 阶段/所有者 | 允许变更建议 | 必须保持的边界 |
| --- | --- | --- |
| DA-1 架构师 | 本报告 | 其他文件只读 |
| DA-2 后端实施，先持独占写权 | 新 `server/pkg/agent/deepagents.go`；`agent.go` SupportedTypes/New/launch header；`launch.go` argv guards；`models.go` 发现；必要的 `hermes.go`/`acp_session.go` 窄扩展及同目录测试 | 不扩 Backend API，不复制 agent loop，不改其他 provider 语义 |
| DA-2 后端实施，同一串行阶段 | `server/internal/daemon/agents_probe.go` 新 `MULTICA_DEEPAGENTS_PATH`/`_MODEL` + dcode probe；`config.go` command hints/列表；`daemon.go` 会话状态根环境注入及恢复策略；`execenv/runtime_config.go` AGENTS；`execenv/context.go` skills；新增专用状态路径 helper 及测试；`runtime_mcp.go` 明确 passthrough/inventory 不重复导入 | 不修改现有 daemon 配置、不启动服务；参数环境需穿透自定义 profile |
| DA-2 后端实施 | 新 migration 扩 runtime_profile CHECK 至 deepagents；`scripts/agent-cli-command-names.txt` 加 dcode；白名单一致性测试 | 不编辑历史 migration；以当时最大 migration 编号分配；只写迁移，不在现有 DB 执行；无新表、无 FK/索引需求 |
| DA-2 界面/文档实施，在后端释放写权后 | `packages/core/types/agent.ts` family tuple；`packages/core/agents/mcp-support.ts`；`packages/views/runtimes/components/provider-logo.tsx` 显示名/既有通用图标；由 tuple 驱动的 `runtime-profile-catalog.ts`/dialog 验证；运行时安装/使用 docs 与相关 builtin skill references | 使用现有 selector，不另造页面；模型/不支持能力诚实呈现；英文/中文产品文案先读 conventions；移动端若需改文件先读其 CLAUDE |
| DA-3 测试 | 新 Deep Agents fake subprocess、真实 SDK smoke 和路径/MCP/恢复测试及证据文档 | 测试代码写入仍串行；默认测试不执行已安装的 dcode，不用已有 DB/端口/账户 |
| DA-4 独立审查 | 默认只读，发现按文件行号回报 | 不自行扩大实现范围，不启动 OpenAI |

`BuiltinRuntimes` 当前用于已有 family 的衍生 runtime；Deep Agents 需要新的行为 family，不能仅加一个 Hermes descriptor 来绕过白名单。后端 handler `runtime_profile.go:154` 使用 SupportedTypes，前端 tuple 和数据库 CHECK 必须同步，否则会发生“能发现但不能创建 profile”。自动发现只是可执行文件可达，不代表模型授权可用；UI 应允许绑定选用，执行阶段给出明确缺少模型配置错误。

## 7. DA-2/DA-3 验收测试契约

默认测试以测试创建的绝对路径假进程为入口，不能查找用户的 CLI。为每次进程建隔离 cwd/profile；不运行 `make up/dev`，不接 Multica 服务数据库。

1. **启动/参数**：默认 dcode --acp、路径含空格、fixed/extra/custom 次序、model 参数、冲突拒绝、环境优先级；记录脱敏 argv；验证实际 SQLite 路径位于测试 profile 的 `.state/sessions.db`，不能写入默认全局 profile。
2. **RPC**：捕获 initialize/new/prompt 三段真实 wire 形状，绝对 cwd；不兼容版本、缺 session ID、乱序 response、畸形 stdout、提前 EOF 全部失败且单一 Result，无 goroutine/子进程泄漏。
3. **消息/终态**：text/thinking/tool call/update、重复 ID、子 agent 并行工具、失败工具、尾部 chunk；Output 不把工具前叙述当最终结论；上述 stopReason 全矩阵；`TerminalObserved` 如实现必须在有权威终态时置位，且先于 Result。
4. **MCP**：检查临时文件权限/内容/清理；session mcpServers 为空；本地独立 stdio MCP 的 tools/list 和 tools/call 在真实 SDK 中可见；新建和 load 后各调用一次；错误配置不能默默变成无工具运行；HTTP/SSE 用单独本地端口临时进程验证或明确未支持。
5. **恢复**：相同 cwd+profile，第一进程完成，第二进程 load 并继续；保留旧上下文、回放不重复显示/计数；缺失 ID、错误 cwd、load capability 缺失、数据库不可达、网络/MCP/auth 失败分别断言；取消不触发 fresh retry。中途杀进程场景明确副作用未知，不承诺 exactly-once。
6. **取消**：prompt pending 时确实收到无 id cancel；合作 cancelled 与不响应两条路径；后者验证 deadline 后所有本次 owned 子进程退出；结果 aborted/timeout，而非 completed。
7. **上下文/产物**：隔离 AGENTS 中随机 canary 经真实 SDK 模型上下文构造可见（可用测试模型，无需外部密钥）；工具只写测试目录文件并核验 bytes；Multica 准备/附件层 fixture 证明回传不丢路径，不声称已完成在线附件上传。
8. **发现/UI**：fake dcode 在指定 PATH 被发现并注册 deepagents；factory、handler 白名单、DB CHECK 文本和 TS tuple 一致；profile 创建、runtime picker 绑定、MCP 配置入口及 model 手填/目录回退闭环。

推荐先运行窄包测试，再按风险扩展：`go test ./pkg/agent -run 'DeepAgents|ACP' -count=1`；daemon/execenv 使用 `-run` 限定新增**无数据库**测试；前端使用现有 Vitest 命令跑新增 catalog/picker/helper 用例。这些是待执行命令，本报告不声称通过。

真实 agent smoke 保留仓库要求的 `agentintegration` tag + `MULTICA_RUN_REAL_AGENT_SMOKE=1` 双门禁，使用独立 venv 的绝对 dcode 路径和新建 profile/cwd。先做版本、导入和无凭据启动检查；没有提供者凭据时只报告实际 startup/config 错误，不当作模型 E2E PASS。可先用真实 `AgentServerACP` + 可控测试 graph 验证协议/取消/恢复，再以预置 dcode 验证完整入口；前者不能代替后者。

## 8. 未解决事项与交接条件

DA-1 无阻止设计移交的缺口。未执行：依赖安装/导入、真实 dcode 启动、端到端模型/MCP/上下文/文件操作、SQLite 跨进程恢复、Go/TS 测试。未检查或借用任何模型凭据；模型 E2E 的授权配置可用性未知，若 DA-3 仍缺失，应将该验收项报告 BLOCKED。

实施的重点风险是 dcode MCP 配置合并、profile 到实际数据库的路径绑定、load 错误 fixture 与取消后的未知工具副作用。后端无需再次选择协议或重写循环；按本报告的新 family + 复用 transport + MCP 私有配置 + capability-gated load 实现即可。DA-2/DA-3/DA-4 均须由 leader 在前阶段交付后明确派发；本报告不改变 squad 父任务状态。
