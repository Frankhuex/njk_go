# NJK Go 记忆系统设计方案

## 1. 文档目的

本文档描述如何在当前 `njk_go` 仓库中引入一套类似 `mem0` 的长期记忆系统。目标不是直接落代码，而是给后续实现提供统一设计依据，包括：

- 记忆系统在当前项目中的职责边界
- 建表方案与索引方案
- `gorm/gen` 模型生成方式
- 数据查询与召回策略
- 各模块的目录落点与调用链
- 分阶段实施建议

本文档基于当前仓库实际结构整理，参考了以下现状：

- 在线服务入口：`cmd/server/main.go`
- 业务核心：`internal/service/`
- 数据访问：`internal/client/pgstore/pgstore.go`
- AI 调用：`internal/client/ai/ai.go`
- 消息入库：`internal/service/save_message.go`
- AI 回复：`internal/service/command_ai.go`
- 启动和说明文档：`AGENTS.md`、`README.md`、`run.sh`
- 参考实现：`build/mem0.py`

## 2. 当前项目现状与接入原则

### 2.1 当前主链

当前项目的主运行链路是：

1. `cmd/server/main.go` 读取配置
2. `pgstore.InitStore(cfg.DSN())` 初始化 PostgreSQL
3. `service.NewService(...)` 装配业务依赖
4. `ws.NewServer(...)` 启动 NapCat WebSocket 服务
5. `handler/napcat` 负责 NapCat 入站事件分发
6. `service` 负责消息入库、命令、AI 回复、图片和表情等业务

### 2.2 当前与记忆系统最相关的已有能力

- `message` 表已经保存了群消息的原始事实流
- `user`、`group`、`at_user` 提供了基础实体关系
- `service.SaveIncomingMessageAndCheckImages(...)` 已经是“消息规范化并成功入库”的稳定入口
- `service.GenerateNJKReply(...)`、`handleAICommand(...)`、`handleAICCommand(...)` 已经是 AI 上下文拼装入口
- `internal/client/ai/ai.go` 已有 OpenAI-compatible chat completions 能力
- `internal/client/pgstore/pgstore.go` 已统一承担数据库读写

### 2.3 接入原则

- 不改造现有 `message` 和 `user` 的主语义
- 新建独立记忆表，复用现有 `message_id`、`group_id`、`user_id`
- `pgstore` 负责记忆表的读写和向量检索
- `service` 负责“该不该写记忆、该查哪些记忆、如何拼 prompt”的业务编排
- `handler` 不承担记忆路由和记忆策略
- AI client 继续只负责模型请求，不承担业务编排

## 3. 参考 `mem0.py` 的设计取舍

`build/mem0.py` 里最值得借鉴的不是它的具体类结构，而是它的“阶段化记忆流水线”：

1. 收集最近上下文
2. 先查已有记忆
3. 让大模型抽取可写入的记忆
4. 批量生成 embedding
5. 做 hash 去重
6. 持久化到向量存储
7. 可选地做 entity linking

结合当前 Go 项目，建议吸收以下思想：

- 写入前先检索，避免把同类记忆反复写入
- LLM 只负责“抽取和决策”，不直接负责数据库落地
- embedding 与写入应支持批量
- 检索必须带作用域过滤，不能做全库裸搜
- entity store 是可选增强，不是 V1 必需能力

不建议直接照搬的部分：

- 不需要额外引入一个独立的历史 SQLite
- 不需要一开始就实现通用 metadata 过滤 DSL
- 不需要一开始就做完整 entity store
- 不需要一开始就支持所有 CRUD 和复杂 rerank

## 4. 设计目标

### 4.1 V1 目标

V1 只做两类长期记忆：

- `memory_fact`：事实记忆
- `memory_impression`：人员印象记忆

V1 要解决的问题：

- 从群聊消息中抽取值得长期保存的信息
- 为后续 `.ai`、`.aic`、自动 `NJK` 回复提供长期上下文
- 在群维度内做 scoped retrieval，而不是全局检索
- 支持基于 `pgvector` 的语义召回

### 4.2 V1 非目标

以下能力不建议在第一期就做：

- 单独的实体库 `memory_entity`
- 复杂的记忆冲突自动合并
- 多阶段 rerank pipeline
- 完整记忆管理后台
- 用户级显式增删改查命令
- 图片 embedding、多模态记忆

## 5. 记忆数据模型

### 5.1 为什么不能直接复用 `message`

`message` 是原始消息流水，适合保存“发生了什么”，不适合直接承担“总结后的长期记忆”：

- 一条消息里可能没有长期价值
- 一条消息里可能抽出多条记忆
- 记忆文本通常不是原文，而是抽取和归纳后的结果
- 记忆需要单独的 embedding、置信度、版本、去重策略

所以 `message` 应继续作为来源表，而不是记忆表本体。

### 5.2 事实记忆表 `memory_fact`

用途：

- 存放可直接检索的事实性长期记忆
- 既支持“某人相关事实”，也支持“群公共事实”

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | `BIGSERIAL` | 主键 |
| `group_id` | `VARCHAR(30)` | 作用域，关联现有 `group` |
| `user_id` | `VARCHAR(30) NULL` | 可空；为某个用户相关时填写，否则表示群公共事实 |
| `message_id` | `VARCHAR(30) NULL` | 来源消息 ID，关联现有 `message` |
| `content` | `TEXT NOT NULL` | 记忆正文 |
| `content_hash` | `VARCHAR(32)` | 记忆文本 hash，用于去重 |
| `embedding` | `VECTOR(n)` | 语义向量 |
| `confidence` | `REAL` | 抽取可信度 |
| `is_active` | `BOOLEAN` | 当前是否有效 |
| `created_at` | `TIMESTAMPTZ` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 更新时间 |

说明：

- `user_id` 建议保留为可空字段，因为很多事实是“某人相关”，保留后可提升检索精度
- `message_id` 很重要，用于溯源、解释、去重和后续管理
- `content_hash` 适合做轻量去重，不依赖每次都走向量比对

### 5.3 人员印象表 `memory_impression`

用途：

- 存放对某个用户的长期印象、偏好、风格、性格判断
- 语义上它不是原始事实，而是对多条事实的抽象

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | `BIGSERIAL` | 主键 |
| `group_id` | `VARCHAR(30)` | 作用域，关联现有 `group` |
| `user_id` | `VARCHAR(30)` | 被描述的用户 |
| `message_id` | `VARCHAR(30) NULL` | 触发这条印象更新的消息 |
| `content` | `TEXT NOT NULL` | 印象正文 |
| `content_hash` | `VARCHAR(32)` | 文本 hash |
| `embedding` | `VECTOR(n)` | 语义向量 |
| `confidence` | `REAL` | 置信度 |
| `version` | `INTEGER` | 版本号 |
| `is_active` | `BOOLEAN` | 当前是否是有效版本 |
| `created_at` | `TIMESTAMPTZ` | 创建时间 |
| `updated_at` | `TIMESTAMPTZ` | 更新时间 |

说明：

- `memory_impression` 和 `memory_fact` 最大区别不是字段命名，而是语义。两张表都统一使用 `user_id` 和 `message_id`，但 `memory_fact.user_id` 表示“该事实关联的用户”，`memory_impression.user_id` 表示“被描述和被刻画的用户”
- `version` 建议保留，因为人物印象会随着时间被修正
- 第一版不一定做“同一用户只保留一条印象”，可以允许多条并存，再由服务层做 top-k 召回

### 5.4 维度 `n` 如何确定

`VECTOR(n)` 中的 `n` 必须和 embedding 模型输出维度一致，例如：

- `1024`
- `1536`
- `768`

设计时必须把 embedding 模型定下来之后再定 DDL。不要先写死维度再临时换模型。

## 6. DDL 建议

下面给出 V1 推荐 DDL 草案。这里是设计文档中的建议结构，后续实现时可以按实际 embedding 维度替换 `VECTOR_DIM`。

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_fact (
    id BIGSERIAL PRIMARY KEY,
    group_id VARCHAR(30) NOT NULL REFERENCES "group"(group_id) ON DELETE CASCADE,
    user_id VARCHAR(30) NULL REFERENCES "user"(user_id) ON DELETE SET NULL,
    message_id VARCHAR(30) NULL REFERENCES message(message_id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    content_hash VARCHAR(32) NOT NULL,
    embedding VECTOR(VECTOR_DIM) NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS memory_impression (
    id BIGSERIAL PRIMARY KEY,
    group_id VARCHAR(30) NOT NULL REFERENCES "group"(group_id) ON DELETE CASCADE,
    user_id VARCHAR(30) NOT NULL REFERENCES "user"(user_id) ON DELETE CASCADE,
    message_id VARCHAR(30) NULL REFERENCES message(message_id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    content_hash VARCHAR(32) NOT NULL,
    embedding VECTOR(VECTOR_DIM) NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## 7. 索引设计

### 7.1 `memory_fact` 必需索引

建议至少加以下索引：

```sql
CREATE INDEX IF NOT EXISTS idx_memory_fact_group_id
    ON memory_fact (group_id);

CREATE INDEX IF NOT EXISTS idx_memory_fact_group_user
    ON memory_fact (group_id, user_id);

CREATE INDEX IF NOT EXISTS idx_memory_fact_source_message
    ON memory_fact (message_id);

CREATE INDEX IF NOT EXISTS idx_memory_fact_group_hash
    ON memory_fact (group_id, content_hash);

CREATE INDEX IF NOT EXISTS idx_memory_fact_embedding_hnsw
    ON memory_fact USING hnsw (embedding vector_cosine_ops);
```

建议补充一个部分唯一索引用于防止同 scope 下反复写入相同文本：

```sql
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_fact_active_hash
    ON memory_fact (group_id, COALESCE(user_id, ''), content_hash)
    WHERE is_active = TRUE;
```

### 7.2 `memory_impression` 必需索引

```sql
CREATE INDEX IF NOT EXISTS idx_memory_impression_group_user
    ON memory_impression (group_id, user_id);

CREATE INDEX IF NOT EXISTS idx_memory_impression_source_message
    ON memory_impression (message_id);

CREATE INDEX IF NOT EXISTS idx_memory_impression_group_hash
    ON memory_impression (group_id, user_id, content_hash);

CREATE INDEX IF NOT EXISTS idx_memory_impression_embedding_hnsw
    ON memory_impression USING hnsw (embedding vector_cosine_ops);
```

如果希望同一时刻每个用户只有一条当前主印象，可增加：

```sql
CREATE UNIQUE INDEX IF NOT EXISTS uq_memory_impression_current
    ON memory_impression (group_id, user_id, content_hash)
    WHERE is_active = TRUE;
```

### 7.3 为什么推荐 `hnsw`

对于 `pgvector`，在当前场景下优先推荐：

- 使用 `hnsw`
- 使用 `vector_cosine_ops`

原因：

- 语义文本检索通常更适合 cosine
- `hnsw` 作为长期在线检索索引更省心
- 当前项目不是超大批量离线导入为主，更适合先用 `hnsw`

如果未来写入量极大、批量导入更频繁，再评估 `ivfflat`。

## 8. 记忆写入策略

### 8.1 写入触发点

最推荐的 V1 触发点：

- 用户消息：`internal/service/save_message.go` 在 `SaveMessage(...)` 成功之后
- 机器人自发消息：`saveSelfMessage(...)` 成功之后

这样做的好处：

- 只对成功落库的消息做记忆派生
- 可以直接复用 `message_id` 作为来源锚点
- 不会把解析失败或发送失败的内容写进记忆

### 8.2 命令消息的特殊性

当前项目里“命中命令的消息不一定入库”，这是已有行为。

这意味着：

- 如果完全依赖 `SaveIncomingMessageAndCheckImages(...)` 之后再写记忆
- 那么部分命令触发型消息可能不会进入长期记忆

V1 有两种策略：

#### 策略 A：保持现状

- 只对已入库消息写记忆
- 实现最简单
- 但会漏掉一部分用户显式提问或意图

#### 策略 B：补充命令消息记忆入口

- 在 `handler/napcat` 命令匹配前后增加“命令文本是否应记忆”的判断
- 或调整“命中命令也允许入库”的策略

建议：

- V1 先采用策略 A
- 等记忆系统跑稳后，再评估是否把命令消息纳入

### 8.3 写入决策流程

建议的服务层写入流水线如下：

1. 读取当前消息及其基础作用域
2. 做规则过滤
3. 用 LLM 决策是否值得写记忆
4. 如果值得写，生成结构化结果
5. 对记忆文本生成 embedding
6. 先按 hash 和近邻查重
7. 写入 `memory_fact` / `memory_impression`

### 8.4 规则过滤建议

在调用 LLM 之前先做 cheap filter，避免每条消息都调用模型：

- 纯表情、纯图片、纯 CQ 码跳过
- 文本太短时跳过
- 明显问候、水群废话可直接跳过
- 重复刷屏内容可跳过
- 机器人自己无信息增量的回复可跳过

### 8.5 LLM 写入决策建议

建议让大模型输出结构化 JSON，包含：

- 是否写入
- 记忆类型：`fact` / `impression` / `none`
- 目标用户
- 记忆文本
- 置信度
- 可选原因

建议原则：

- 事实记忆只抽“未来有检索价值”的稳定事实
- 人员印象只抽“长期倾向、偏好、风格、关系”
- 不要把一次性上下文、临时情绪、纯闲聊都写进去

### 8.6 去重策略

V1 建议双层去重：

- 第一层：`content_hash`
- 第二层：同 scope 下做近邻检索，若相似度过高则跳过或改为更新

建议阈值：

- 事实记忆：相似度高于 `0.92` 可视为重复
- 人员印象：相似度高于 `0.88` 可视为同类印象，适合版本更新而非新增

## 9. 记忆读取策略

### 9.1 读取触发点

最适合接入的现有位置：

- `internal/service/command_ai.go` 中 `handleAICommand(...)`
- `internal/service/command_ai.go` 中 `handleAICCommand(...)`
- `internal/service/command_ai.go` 中 `GenerateNJKReply(...)`

也就是在 `aiClient.Complete(...)` 之前，先召回长期记忆，再拼入 prompt。

### 9.2 读取流程

建议读取链路如下：

1. 根据当前问题或最近消息构造 retrieval query
2. 生成 query embedding
3. 先查 `memory_fact`
4. 再查 `memory_impression`
5. 对召回结果按阈值和 top-k 过滤
6. 拼成一段结构化记忆上下文
7. 连同最近消息窗口一起送入大模型

### 9.3 检索作用域

记忆检索绝不能做全库裸搜。建议强制至少带：

- `group_id`

然后按场景补充：

- `user_id`

推荐策略：

- `memory_fact`：先召回 `user_id = 当前说话人` 的个人事实，再召回 `user_id IS NULL` 的群公共事实
- `memory_impression`：如果消息中包含明确说话人或提及对象，优先查这些人的印象

### 9.4 Top-K 建议

V1 建议：

- 事实记忆取 `5` 到 `8` 条
- 人员印象取 `1` 到 `3` 条

不要把过多记忆直接塞进 prompt，否则会：

- 抬高 token 成本
- 干扰近期聊天上下文
- 增加模型被旧记忆带偏的风险

### 9.5 Prompt 拼装建议

建议把长期记忆作为独立区块，而不是混进聊天记录正文。可以分三段：

1. 系统提示词
2. 长期记忆摘要块
3. 最近聊天记录

记忆块建议强调：

- 这些记忆可能不完全准确
- 仅在相关时参考
- 新上下文与旧记忆冲突时，以当前上下文优先

## 10. 查询方案

### 10.1 为什么继续推荐手写 SQL

当前 `pgstore` 已经大量使用手写 GORM 查询和 `Raw SQL`，而不是依赖 generated query DSL。

对于 `pgvector` 检索，这样做更自然，因为：

- 需要直接使用 `<=>` 这类向量距离操作符
- 需要手写 similarity score
- 需要做 scope filter、active filter、top-k、阈值判断

所以记忆查询建议继续放在 `internal/client/pgstore/` 中，以手写 SQL 或 `db.Raw(...)` 方式实现。

### 10.2 事实记忆查询建议

查询原则：

- 强制 `group_id`
- 过滤 `is_active = TRUE`
- 先按 `user_id = ?` 查个人事实
- 再按 `user_id IS NULL` 查公共事实
- 使用 cosine distance 排序

返回字段建议包含：

- `id`
- `content`
- `message_id`
- `confidence`
- `similarity`

### 10.3 人员印象查询建议

查询原则：

- 强制 `group_id`
- 强制 `user_id`
- 只查 `is_active = TRUE`
- 按向量相似度排序
- 可按 `version DESC` 做轻量偏置

### 10.4 相似度阈值

V1 建议不要把阈值写死在 SQL 里，而是由服务层可配置：

- `memory_fact_min_similarity`
- `memory_impression_min_similarity`

原因：

- 不同 embedding 模型的分数分布差异很大
- 早期调参频率会比较高

### 10.5 查询结果的服务层再筛选

数据库层只做第一层召回，服务层再做：

- 去重
- 截断
- 按来源消息和用户去重
- 按类型分组
- 拼装成 prompt 文本

## 11. `gorm/gen` 模型生成方案

### 11.1 当前现状

当前仓库有：

- 生成入口：`cmd/gen-gorm/main.go`
- 生成脚本：`gen_gorm.sh`
- 生成结果目录：`internal/dal/model/`、`internal/dal/query/`

当前 `cmd/gen-gorm/main.go` 中负责配置生成输出目录：

- `./internal/query`
- `./internal/model`

而真实仓库目录是：

- `./internal/dal/query`
- `./internal/dal/model`

因此，在给记忆表生成 model 之前，建议先确认 `cmd/gen-gorm/main.go` 中的输出路径配置，再执行生成。

### 11.2 记忆表模型生成步骤

建议步骤如下：

1. 在 SQL 文件中加入 `memory_fact`、`memory_impression` 及索引 DDL
2. 在数据库中执行建表
3. 确认 `cmd/gen-gorm/main.go` 的输出目录与 `internal/dal/` 保持一致
4. 为 `vector` 类型增加自定义类型映射
5. 执行 `sh gen_gorm.sh`
6. 检查 `internal/dal/model/` 下是否生成对应 model

### 11.3 `vector` 类型的处理建议

`pgvector` 的 `vector` 列通常不能指望 `gorm/gen` 自动推断成理想 Go 类型。设计上建议：

- 在生成器中显式配置 `vector` 到自定义 Go 类型的映射
- Go 类型建议统一使用 `pgvector-go` 提供的类型
- 不建议把向量列映射成裸字符串或 `[]byte`

原因：

- 后续插入和查询都要直接与 `pgvector` 交互
- 使用专门的向量类型更稳定，可读性也更好

### 11.4 为什么不推荐全部依赖 generated query

即使生成了 model/query，记忆检索层仍建议：

- model 用 generated model
- 查询用 `pgstore` 手写 SQL

因为：

- 向量检索 SQL 可读性更强
- 更容易调试相似度、阈值、排序
- 与当前仓库 `pgstore` 风格一致

## 12. 模块设计

### 12.1 总体分层

建议保持当前分层风格：

- `client`：只做外部依赖和数据访问
- `service`：只做记忆编排
- `handler`：不碰记忆策略

### 12.2 `internal/client/pgstore`

新增职责：

- 写入事实记忆
- 写入人员印象
- 近邻查重
- 事实向量检索
- 印象向量检索
- 记忆版本更新或失活

建议文件拆分：

- `internal/client/pgstore/pgstore.go`
  - 保持 `Store` 定义和初始化
- `internal/client/pgstore/memory_fact.go`
  - 事实记忆相关查询
- `internal/client/pgstore/memory_impression.go`
  - 印象记忆相关查询

### 12.3 `internal/client/ai`

当前该目录只有 chat completions。结合你已经新增的 `EMBED_MODEL_NAME`，本项目更适合继续沿用 `internal/client/ai/`，在同一个 client 下同时支持：

- chat completions
- embeddings

推荐原因：

- 当前仓库已经有 `internal/client/ai/ai.go`
- 如果聊天模型和嵌入模型走同一个 OpenAI-compatible 服务，复用同一个 `baseURL`、`apiKey` 和 `httpClient` 最自然
- 可以减少新增目录和额外装配成本

推荐的改动方向如下。

#### 12.3.1 配置层改动

在当前配置基础上新增：

- `EMBED_MODEL_NAME`

约定：

- `BASE_URL` 继续作为 chat 和 embedding 共用的 OpenAI-compatible 基础地址
- `API_KEY` 继续复用
- `MODEL_NAME` 用于聊天模型
- `EMBED_MODEL_NAME` 用于嵌入模型

如果未来 embedding 单独走其他供应商，再考虑增加：

- `EMBED_BASE_URL`
- `EMBED_API_KEY`

但从当前项目状态看，第一版没有必要先做复杂化。

#### 12.3.2 Client 结构改动

当前 `AIClient` 只有：

- `baseURL`
- `apiKey`
- `modelName`
- `httpClient`

为了支持嵌入模型，建议把它扩展为同时持有：

- chat model name
- embed model name

推荐思路：

- `modelName` 继续服务 chat completions
- 新增 `embedModelName`
- 继续共用现有 `httpClient`

这样后续 `service` 不需要再注入第二个 embedding client。

#### 12.3.3 接口层改动

当前 `service` 侧只依赖：

- `Complete(ctx, systemPrompt, userPrompt, temperature)`

为了支持记忆系统，建议在 `internal/client/ai` 增加嵌入能力接口，例如语义上增加：

- 单文本 embedding
- 批量文本 embedding

设计目标：

- `service` 在写记忆时可对抽取后的记忆文本做 embedding
- `service` 在读记忆时可对查询文本做 embedding
- `pgstore` 可以接收 embedding 结果用于插入和检索

#### 12.3.4 HTTP API 改动

当前 `ai.go` 只调用：

- `/chat/completions`

支持 embedding 后，还需要调用：

- `/embeddings`

建议保持同样的风格：

- 继续用同一个 `httpClient`
- 继续走 `Authorization: Bearer ...`
- 继续由 `baseURL` 拼接 endpoint

#### 12.3.5 文件拆分建议

为了避免 `ai.go` 继续膨胀，建议拆成：

- `chat.go`
  - chat completions 请求与响应
- `embed.go`
  - embeddings 请求与响应
- `client.go`
  - `AIClient` 结构和构造函数
- `types.go`
  - 公共请求响应结构

如果你倾向于维持少文件，也可以先继续单文件实现，但从设计上建议至少按 chat/embed 拆开。

#### 12.3.6 Service 依赖改动

当前 `service.Service` 里已经有：

- `aiClient`
- `freeAIClient`

记忆系统接入后，推荐优先复用现有 `aiClient` 作为“聊天 + embedding”双能力 client，而不是再额外加一个独立字段。

原因：

- 你的配置已经通过 `EMBED_MODEL_NAME` 指定了嵌入模型
- 同一个 client 更容易共用认证、超时、base URL
- `service.NewService(...)` 的构造复杂度更低

如果未来 embedding 供应商独立，再考虑把 `Embedder` 抽成单独依赖接口。

### 12.4 `internal/service`

建议新增一组记忆编排文件：

- `memory.go`
  - 对外主入口
- `memory_write.go`
  - 写入决策和写入流水线
- `memory_read.go`
  - 读取检索与 prompt 拼装
- `memory_prompts.go`
  - 记忆提取和记忆使用提示词

职责边界：

- `service` 负责 deciding 和 orchestration
- 不在这里写 SQL
- 不在这里直接组 HTTP 请求

### 12.5 配置模块

当前配置在 `internal/config/config.go`。结合你当前的配置约束，记忆系统建议新增配置项：

- `EMBED_MODEL_NAME`
- `MEMORY_ENABLED`
- `MEMORY_FACT_TOPK`
- `MEMORY_IMPRESSION_TOPK`
- `MEMORY_FACT_MIN_SIMILARITY`
- `MEMORY_IMPRESSION_MIN_SIMILARITY`

其中：

- `BASE_URL` 复用现有 OpenAI-compatible 服务地址
- `API_KEY` 复用现有认证
- `MODEL_NAME` 继续作为聊天模型
- `EMBED_MODEL_NAME` 作为嵌入模型

如果未来 embedding 要独立部署，再补充 `EMBED_BASE_URL` / `EMBED_API_KEY` 即可。

## 13. 建议的服务接口形态

本文档不写实现代码，但建议服务层至少具备以下几类能力：

- 对一条已入库消息尝试写记忆
- 为一次 AI 回复请求构建长期记忆上下文
- 为事实库和印象库分别做检索
- 对记忆写入做规则过滤和去重

建议思路：

- `Service` 继续作为统一业务入口
- 不额外造一层 `MemoryService struct`
- 直接在 `service` 包下挂一组方法

这样更符合当前仓库已经确立的风格。

## 14. 记忆写入调用链设计

### 14.1 用户消息

建议调用链：

1. `handler/napcat.HandleGroupMessage(...)`
2. `service.SaveIncomingMessageAndCheckImages(...)`
3. `store.SaveMessage(...)`
4. `service.TryWriteMemoriesFromMessage(...)`

说明：

- 记忆写入应发生在主消息已成功入库之后
- 推荐异步或弱耦合调用，避免记忆失败影响主链

### 14.2 机器人回复

建议调用链：

1. `handler` 发送消息
2. NapCat 返回 `action_response`
3. `service.CompleteActionResult(...)`
4. `saveSelfMessage(...)`
5. `service.TryWriteMemoriesFromSelfMessage(...)`

说明：

- 只对真正发送成功并已落库的机器人消息做记忆
- 如果担心自我污染，V1 可以先只给用户消息写记忆

## 15. 记忆读取调用链设计

建议调用链：

1. `.ai` / `.aic` / 自动 `NJK` 准备调用 `aiClient.Complete(...)`
2. `service.BuildMemoryContext(...)`
3. `pgstore.SearchFactMemories(...)`
4. `pgstore.SearchImpressionMemories(...)`
5. `service.ComposeMemoryPromptBlock(...)`
6. 把记忆块拼进 `userPrompt`
7. 调用 `aiClient.Complete(...)`

## 16. Prompt 设计建议

### 16.1 写入提示词

写入提示词建议要求模型输出结构化结果，明确：

- 哪些内容值得长期保存
- 是事实还是人员印象
- 与哪个用户相关
- 置信度大概是多少

注意事项：

- 必须强调“宁缺毋滥”
- 不要提取纯临时语境
- 不要把一句普通闲聊硬提炼成长期人格判断

### 16.2 读取提示词

读取提示词不一定需要单独调用一个模型做 rerank。V1 可以直接把检索结果拼进去，告诉主模型：

- 下面是可能相关的长期记忆
- 如与当前上下文冲突，以当前上下文为准
- 只在相关时参考，不要机械复述

### 16.3 与现有 AI prompt 的关系

当前 `.ai` / `NJK` 已有自己的系统提示词。记忆系统不建议替换它们，而是作为一个额外块插入：

- 系统提示词保持原样
- 用户 prompt 中增加“长期记忆”区块
- 最近消息窗口仍然保留

## 17. 版本与更新策略

### 17.1 `memory_fact`

事实记忆建议优先：

- 新增
- 重复则跳过

不建议 V1 做复杂覆盖更新，因为事实通常可并列存在。

### 17.2 `memory_impression`

人员印象建议允许：

- 新增新版本
- 旧版本标记为 `is_active = FALSE`
- 最新版本 `version + 1`

这样更稳，因为人物印象会随着上下文变化被修正。

## 18. 失败处理与性能策略

### 18.1 失败隔离

记忆系统失败不应影响主聊天链路：

- 消息入库成功后，即使记忆提取失败，也不能回滚主消息
- AI 回复过程中，即使记忆检索失败，也应降级为“只用近期聊天记录”

### 18.2 同步还是异步

V1 建议：

- 读取链路同步
- 写入链路可先同步实现，再逐步改成异步

原因：

- 读取链路必须即时返回给模型
- 写入链路对时延更敏感，后续可以异步化

### 18.3 批量策略

如果后续想回填历史消息，建议：

- 先按时间窗口批量读取 `message`
- 批量跑抽取
- 批量 embedding
- 批量写入

这部分可以借鉴 `mem0.py` 的 phased batch pipeline。

## 19. V1 推荐实施顺序

### 第一阶段：数据库和模型

1. 确认 embedding 模型维度
2. 在 SQL 中加入两张记忆表和索引
3. 确认 `cmd/gen-gorm/main.go` 的输出目录
4. 为 `vector` 类型补充生成映射
5. 生成 gorm model

### 第二阶段：基础存储接口

1. 在 `pgstore` 中补充记忆写入接口
2. 在 `pgstore` 中补充记忆检索接口
3. 先用手写 SQL 跑通向量检索

### 第三阶段：服务写入链

1. 在消息成功入库后调用记忆写入流程
2. 先做规则过滤
3. 再接入 LLM 决策和 embedding
4. 做基础去重

### 第四阶段：服务读取链

1. 在 `.ai`
2. 在 `.aic`
3. 在自动 `NJK` 回复

分别接入长期记忆检索与 prompt 拼接。

### 第五阶段：调参与治理

1. 调整阈值和 top-k
2. 调整写入提示词
3. 清理低质量记忆
4. 再考虑 impression 版本更新和 entity store

## 20. 推荐的最小落地范围

如果希望最稳地开始，建议只做这一套最小子集：

- 两张表：`memory_fact`、`memory_impression`
- `pgvector` + `hnsw`
- 用户消息写记忆
- `.ai` 和自动 `NJK` 读记忆
- `pgstore` 手写 SQL 检索
- `service` 编排写入和读取
- `internal/client/ai` 新增 embedding 能力

先不要做：

- entity store
- 后台管理
- 命令消息记忆
- 自我记忆
- 多模态记忆

## 21. 结论

这套设计与当前仓库的现有结构是兼容的：

- `message` 继续做原始语料来源
- `pgstore` 继续做数据库访问中心
- `service` 继续做业务编排中心
- `handler` 不承担记忆路由
- `ai client` 继续只做模型调用

参考 `mem0.py` 后，当前项目最值得吸收的是：

- 写入前先做检索与去重
- 抽取、embedding、持久化分阶段处理
- 检索必须严格按 scope 过滤

而最适合当前仓库的本地化落地方式是：

- 不照搬完整 `mem0` 框架
- 只引入“事实库 + 人员印象库”两张表
- 让 `service` 在现有 `.ai` 和消息入库链路中接入记忆读写

这会是当前 `njk_go` 最小、最稳、也最符合现有代码风格的一版长期记忆系统。
