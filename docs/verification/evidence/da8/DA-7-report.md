AH-13 / DA-7 QA：**FAIL（名称唯一性检查）；单 broker TLS→官方 loader 与指定回归 PASS。** 模型/外部环境等仍 BLOCKED。未修改源码、状态或服务。

候选完整 HEAD `3af7beca4148e444457592645501c46c83dd067f`，origin 为 `CM-BF/multica-runtime-lab`；测试前后 HEAD 一致、工作树干净。runner `da7-run.py` 使用私有 HOME/USERPROFILE/XDG/TMPDIR/GOTMPDIR/GOCACHE/GOPATH，仅复用 Go 模块缓存；显式使用 fork 专用 `.da2-tools/venv/bin/python`，禁 pycache 写入，不读取账户凭据或继承 MULTICA 根配置。TLS 测试按提交实现复制 wrapper 到临时目录；测试副本、缓存和进程均已清理，未安装或改动现有服务。

**发现：P2 名称冲突仍存在，未给整体 PASS。**

`server/internal/daemon/remote_mcp_broker.go:200–205` 将冒号替换为下划线后，只取 ContributionID 前 8 字符。合法 plugin ID 的前缀 `plugin:` 占 7 字符；两个不同安装，例如 `plugin:01a08abc-1111-7111-8111-111111111111:toolbox`、`plugin:01b09def-2222-7222-8222-222222222222:toolbox`，在相同 ContributionKey 下均得到 **`plugin-toolbox-plugin_0`**。`servers[name] = ...`（同文件约第 141 行）会静默覆盖前一个 broker 配置，两个配置的工具/策略不会独立保留。这是原有前缀截断策略缺陷，不能说是本次冒号替换新引入；但修正字符有效性尚不足以证明多连接映射正确。

独立私有 Go overlay 探针 `TestDA7BrokerNames`：字符有效性子项 PASS，distinct-installations 子项 **FAIL，退出 1**，未重试。没有修改产品测试文件。最小建议：对完整 ContributionID 使用足够长度的稳定摘要作为合法后缀，并在最终 map 插入时检测冲突；补同 key/不同安装的配置与策略映射测试。

**指定检查与命令**（完整 argv/环境/退出码及脚本见附件）：

- 名称：`go -C server test -race -overlay <private-overlay> ./internal/daemon -run '^TestDA7BrokerNames$' -count=1 -v -timeout=300s` → **1**，`da7-names.log`。额外使用已安装官方 `_SERVER_NAME_RE` 核验合法新名称通过，而冒号、斜线、空格仍拒绝 → **0**，`da7-validator.log`；没有放宽官方字符规则。
- 双门禁真实 TLS：`go -C server test -race -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealTLSBrokerLoader$' -count=1 -v -timeout=300s` → **0**，2.111s，`da7-tls.log`。缺 CA 拒绝且不启动 loader；私有 CA 下 guard READY、官方 dcode loader 一次初始化；实际工具对象只见 approved `fixture.read`，允许调用经 broker/TLS 得到 fixture-value；直接禁止 write 被策略拒绝。上游 initialize=4、list=4、允许调用=1、禁止调用=0、写入=0，manager cleanup 完成。
- 相关 race：`go -C server test -race ./pkg/remotemcp ./internal/daemon ./pkg/agent -run 'RemoteMCP|DeepAgents|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic' -count=1 -v -timeout=300s` → **0**，37 顶层测试；daemon 1.453s、agent 12.139s。**pkg/remotemcp 在该正则下无测试匹配**，不计该包运行覆盖。见 `da7-regression.log`。
- 原 plugin loader：`go -C server test -race -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealPluginMCP$' -count=1 -v -timeout=300s` → **0**，8 场景，6.189s，`da7-plugin.log`。真实测试显式 gate=1 与隔离 Python 绝对路径，无跳过冒充。

**历史证据核验 PASS：** 提交内 `docs/verification/evidence/da6/tls.log` 保留官方 server name 拒绝和退出 1；`tls-retry.log` 保留修复后退出 0。与报告“一次失败、唯一重试”记录一致；只能确认保留的执行记录，不能证明日志之外从未运行其他命令。本次独立执行不是后端重试。

边界：Go 1.26.6、Python 3.13.3、dcode 0.1.68、deepagents-acp 0.0.11、wrapper 0.1.0；官方 loader/TLS/broker 真实，upstream 是临时 fixture，直接调用 loader，不是模型或生产 ACP turn。

分别保留 **BLOCKED/未执行**：模型 E2E（无授权可用凭据）；外部 broker 基础设施/凭据刷新；本提交全仓回归；现有配置与历史影响审计。DA-5 全三包通过记录不改写，也不代替当前全仓验收。建议先处理名称冲突再判断该提交整体通过；本轮只报告，不自行修代码或开新一轮。
