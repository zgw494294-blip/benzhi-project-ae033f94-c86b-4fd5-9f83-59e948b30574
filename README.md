# stage-rigging-clearance

`stage-rigging-clearance` 是面向剧院舞台技术团队的吊挂换景安全验证与开演放行工作台。它把吊点载荷、换景提示、安全规则、现场排练、偏差整改、安全复核和不可变放行凭据串成一条可审计流程。

服务仅依赖本地 SQLite 数据库，不连接外部系统。普通运行默认监听 `127.0.0.1:19081`，并把业务数据保存到 `rigging-clearance.db`。SQLite 会启用 WAL、外键和写入等待；聚合通过 `expectedVersion` 实现乐观并发控制，所有写 API 都要求 `idempotencyKey`。

## 构建与运行

下载依赖并构建：

```sh
go mod download
go build ./cmd/server
```

启动服务：

```sh
go run ./cmd/server -addr=127.0.0.1:19081
```

浏览器访问 `http://127.0.0.1:19081/workbench`。可通过 `-db` 指定数据库路径，例如：

```sh
go run ./cmd/server -addr=127.0.0.1:19082 -db=theatre.db
```

如果未显式传入 `-addr`，环境变量 `PORT` 为有效端口号时，服务监听 `127.0.0.1:<PORT>`；否则使用默认地址 `127.0.0.1:19081`。服务不会默认绑定 `0.0.0.0`。

## 业务流程

1. 在工作台创建 `DRAFT` 方案，填写演出、场地限制、负责人和计划开演时间。
2. 编制吊点/设备载荷与换景提示。既可保存单项，也可通过工作台批量区原子提交一个变更集；服务在聚合副本上一次返回全部项目错误，通过后只推进一次聚合版本。
3. 运行安全校验。规则按连接树稳定计算自有动载、后代贡献、级联总量和根节点场地净载；每次运行生成不可覆盖的校验批次，并追踪新增、持续、已消除和因输入修订而过期的问题。
4. 开始有界现场排练，按 cueID 逐条暂存或修正执行结果、实测峰值、偏差和证据，刷新或重启后可断点续录。完成操作直接汇总已保存结果并列出缺失提示；通过后进入 `READY_FOR_REVIEW`，阻断时进入 `REMEDIATING`。
5. 在 `REMEDIATING` 状态按问题来源复验。静态问题必须有受影响输入的新 revision 且目标规则不再命中；现场问题必须提交新的提示重测值与证据。所有通过和失败尝试均追加留存。
6. 安全复核员确认清单后，服务在单一事务中冻结规范快照、推进至 `RELEASED`，签发单调序号凭据。放行后的聚合与凭据由数据库触发器保护，不能更新或删除。

## API 与完整性

JSON API 前缀为 `/api/v1`。写请求使用 `application/json`，请求体最大 1 MiB；版本冲突、幂等键复用冲突和业务规则失败会返回稳定的结构化错误。

`POST /api/v1/rigging-cases/{id}/change-sets` 原子提交最多 250 个吊点和提示项目。`GET /api/v1/rigging-cases/{id}/validation-batches?limit=20` 按时间倒序列出校验批次；同资源下的 `/diff?fromBatchID=...&toBatchID=...` 返回两批问题差异。

`PUT /api/v1/rigging-cases/{id}/rehearsals/{runID}/cue-results/{cueID}` 暂存单条排练结果，原有完成入口汇总持久化进度。`GET /api/v1/rigging-cases/{id}` 返回当前聚合、校验批次、排练进度、整改尝试、审计时间线、冻结快照以及摘要校验结果。

`GET /api/v1/release-certificates?serial=42` 或 `?certificateID=...` 精确检索唯一凭据，并分别报告快照摘要、完整审计链、`CASE_RELEASED` 锚点、冻结版本和聚合放行状态；只有五项全部通过才返回冻结清单。`GET /api/v1/rigging-cases/{id}/integrity?from=0` 可从指定事件断点验证摘要链并报告首个损坏位置。

健康检查路径为 `GET /healthz`。

## 测试与自检

运行全部测试：

```sh
go test ./...
```

运行有界真实 HTTP 自检：

```sh
go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
```

`selfcheck` 使用真实回环监听和与普通服务相同的 HTTP 入口，在内存 SQLite 中完成创建、编制、校验、排练、复核、放行和凭据摘要校验，然后主动关闭服务并退出。
