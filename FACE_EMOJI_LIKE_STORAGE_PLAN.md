# face / emoji_like 入库方案

## 需求理解

本次只做方案设计，不改业务代码。

需要新增两张表：

1. `face`：系统表情 ID 字典表，只有 `face_id VARCHAR(30)`，主键。
2. `emoji_like`：表情回应事件表，包含自增主键 `id SERIAL PRIMARY KEY`，以及 `message_id`、`user_id`、`face_id` 三个 `VARCHAR(30)` 字段。

写入语义建议按以下方式落地：

1. 收到 `group_message` 时，只要消息段里有 `type = "face"`，就把 `data.id` upsert 到 `face`，已存在则跳过。
2. 收到 `post_type = "notice"` 且 `notice_type = "group_msg_emoji_like"` 时，把 notice 中的 `emoji_id` upsert 到 `face`，再向 `emoji_like` 插入一条记录。

这里将“若已存在则不存”理解为 `face` 表去重；`emoji_like` 是事件流水表，默认不做唯一约束，避免重复 notice 被误判为同一事件。如果后续想把 `emoji_like` 做成“每人对每条消息每个表情只保留一条”的状态表，再加唯一约束。

## 当前代码现状

主链路：

1. `internal/transport/ws/server.go` 读取 WebSocket text frame。
2. `internal/napcat/parser.go` 通过 `napcat.ParseInboundMessage` 区分 `group_message`、`notice`、`action_response`。
3. `internal/bot/service_ingress.go` 的 `HandleGroupMessage` 处理群消息。
4. `internal/bot/service_ingress.go` 的 `HandleNotice` 处理 notice。
5. `internal/bot/service_ingest.go` 的 `saveIncomingMessageAndCheckImages` 保存部分入站群消息，并遍历 `event.Message.Segments`。

与本需求相关的现状：

1. `internal/napcat/types.go` 已有 `SegmentTypeFace = "face"`，`MessageSegmentData.ID` 可承载 face id。
2. `NoticeEvent` 已有 `EventNoticeTypeGroupMsgEmojiLike = "group_msg_emoji_like"`、`MessageID`、`UserID`、`OperatorID`、`Likes []EmojiLike`。
3. `EmojiLike` 当前字段是 `EmojiId string`，JSON tag 为 `emoji_id`。
4. `NoticeEvent` 当前没有顶层 `EmojiID string` 字段。如果 NapCat 实际上报是顶层 `emoji_id`，需要补字段。
5. 当前 `HandleNotice` 先判断 `TargetID == SelfID` 才响应，这个逻辑更像 poke/notify 场景。`group_msg_emoji_like` 应该在这个判断之前单独处理，否则可能被直接 return。
6. 当前群消息不是全部都会入库：`HandleGroupMessage` 只有在未命中命令或命中 `commandNJK` 时才调用 `saveIncomingMessageAndCheckImages`。如果只把 face upsert 放进该函数，显式命令消息里的 face segment 会漏掉。
7. 现有 `.sh` 脚本只有 `create_njk_tables.sh`、`run_ws_server.sh`、`build_linux.sh`、`docker-compose.sh`，没有 gorm/gen 生成脚本。
8. 当前 `internal/model` 和 `internal/query` 是 gorm/gen 产物，但主业务大多直接使用 `gorm.DB` + `model`，不是强依赖 query 层。

## SQL 设计

建议追加到 `sql/create_njk_tables.sql`，位置可放在 `image` / `img_whitelist` 附近，或放在消息相关表之后。

```sql
CREATE TABLE IF NOT EXISTS face (
    face_id VARCHAR(30) PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS emoji_like (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(30) NOT NULL,
    user_id VARCHAR(30) NOT NULL,
    face_id VARCHAR(30) NOT NULL REFERENCES face(face_id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_emoji_like_message_id
    ON emoji_like (message_id);

CREATE INDEX IF NOT EXISTS idx_emoji_like_user_id
    ON emoji_like (user_id);

CREATE INDEX IF NOT EXISTS idx_emoji_like_face_id
    ON emoji_like (face_id);

CREATE INDEX IF NOT EXISTS idx_emoji_like_message_user
    ON emoji_like (message_id, user_id);
```

不建议默认给 `emoji_like.message_id` 和 `emoji_like.user_id` 加外键：

1. 当前不是所有群消息都会保存到 `message` 表，命令消息和部分机器人动作可能没有对应记录。
2. notice 里的点赞用户不一定已经存在于 `user` 表。
3. 对 notice 事件来说，丢记录比弱约束更糟。

如果后续确认所有目标消息和用户都会先入库，可以再补外键：

```sql
ALTER TABLE emoji_like
    ADD CONSTRAINT fk_emoji_like_message
    FOREIGN KEY (message_id) REFERENCES message(message_id) ON DELETE CASCADE;

ALTER TABLE emoji_like
    ADD CONSTRAINT fk_emoji_like_user
    FOREIGN KEY (user_id) REFERENCES "user"(user_id) ON DELETE CASCADE;
```

如果后续希望 `emoji_like` 去重，可选唯一索引是：

```sql
CREATE UNIQUE INDEX IF NOT EXISTS uq_emoji_like_message_user_face
    ON emoji_like (message_id, user_id, face_id);
```

本次不建议默认添加该唯一索引，因为需求只明确 `face` 去重，没有明确 `emoji_like` 去重。

## 执行 SQL 的命令

仓库已有建表脚本 `create_njk_tables.sh`，它会读取 `../NJK/.env`，并固定执行 `sql/create_njk_tables.sql`。

更新 SQL 文件后执行：

```bash
sh create_njk_tables.sh
```

脚本会拒绝非 `njk` 数据库：

1. 默认 `DB_NAME=njk`。
2. 如果 `.env` 中 `DB_NAME` 不是 `njk`，脚本会退出。
3. 如果 `DB_USER=postgres`，脚本会改用 `njk` 用户。

## gorm model / query 生成

### 现状

当前仓库没有生成脚本，也没有独立的生成器 `main` 包。

尝试执行：

```bash
go run gorm.io/gen/tools/gentool --help
```

当前会失败，原因是 `gorm.io/gen@v0.3.27` 模块内没有 `gorm.io/gen/tools/gentool` 包。也就是说，不能直接依赖这个命令生成。

### 推荐做法

建议新增一个只用于生成代码的 Go 程序，例如 `tools/gen_gorm/main.go`，使用项目现有依赖 `gorm.io/gen` 连接 PostgreSQL 后生成全部表。

生成器核心逻辑应等价于：

```go
package main

import (
    "log"

    "njk_go/internal/config"

    "gorm.io/driver/postgres"
    "gorm.io/gen"
    "gorm.io/gorm"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }

    db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
    if err != nil {
        log.Fatal(err)
    }

    g := gen.NewGenerator(gen.Config{
        OutPath:      "./internal/query",
        ModelPkgPath: "./internal/model",
        Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
        FieldNullable: true,
    })

    g.UseDB(db)
    g.ApplyBasic(g.GenerateAllTable()...)
    g.Execute()
}
```

然后执行：

```bash
go run ./tools/gen_gorm
```

为了贴合现有脚本风格，建议再新增 `generate_gorm.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

go run ./tools/gen_gorm
```

之后统一用：

```bash
sh generate_gorm.sh
```

预期生成或更新这些文件：

1. `internal/model/face.gen.go`
2. `internal/model/emoji_like.gen.go`
3. `internal/query/face.gen.go`
4. `internal/query/emoji_like.gen.go`
5. `internal/query/gen.go`

预期 model 大致形态：

```go
type Face struct {
    FaceID string `gorm:"column:face_id;primaryKey" json:"face_id"`
}

type EmojiLike struct {
    ID        int32  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
    MessageID string `gorm:"column:message_id;not null" json:"message_id"`
    UserID    string `gorm:"column:user_id;not null" json:"user_id"`
    FaceID    string `gorm:"column:face_id;not null" json:"face_id"`
}
```

## 代码改动方案

### Store 层

在 `internal/bot/store.go` 增加最小必要方法：

```go
func (s *Store) UpsertFace(ctx context.Context, faceID string) error
func (s *Store) SaveEmojiLike(ctx context.Context, messageID string, userID string, faceID string) error
```

`UpsertFace` 使用 `clause.OnConflict{DoNothing: true}`。

`SaveEmojiLike` 建议先 `UpsertFace`，再插入 `model.EmojiLike`。如果希望强一致，可以用一个 transaction 包住这两步。

### 群消息 face 提取

新增一个纯函数，集中处理 segment 提取：

```go
func faceIDsFromSegments(segments []napcat.MessageSegment) []string
```

规则：

1. 只处理 `segment.Type == napcat.SegmentTypeFace`。
2. 使用 `strings.TrimSpace(segment.Data.ID.String())`。
3. 空 ID 跳过。
4. 同一条消息里重复 ID 可以先本地去重，减少无意义 upsert。

为了满足“读取 group message 时遍历到 face 就存”的语义，建议在 `HandleGroupMessage` 通过群白名单和用户黑名单后，立即调用一个 helper：

```go
func (s *Service) saveFacesFromGroupMessage(ctx context.Context, event *napcat.GroupMessageEvent)
```

不要只放在 `saveIncomingMessageAndCheckImages`，否则命令消息中的 face segment 会因为当前入库条件而漏记。

同时，`saveIncomingMessageAndCheckImages` 的 segment switch 建议补 `case napcat.SegmentTypeFace`，把消息文本还原为类似：

```go
textParts = append(textParts, fmt.Sprintf("[CQ:face,id=%s]", id))
```

这不是入库 face 字典所必需，但能让历史消息文本更完整。

### notice 表情回应处理

在 `internal/napcat/types.go` 补充字段：

```go
type NoticeEvent struct {
    // ... existing fields
    EmojiID string `json:"emoji_id,omitempty"`
}
```

保留现有 `Likes []EmojiLike`，因为当前代码已经为该结构预留了字段。

在 `HandleNotice` 开头单独分支：

```go
if event.NoticeType == napcat.EventNoticeTypeGroupMsgEmojiLike {
    s.handleGroupMsgEmojiLikeNotice(ctx, event)
    return
}
```

这个分支必须放在 `TargetID == SelfID` 判断之前，避免表情回应 notice 被 poke/notify 的旧逻辑过滤掉。

新增处理函数：

```go
func (s *Service) handleGroupMsgEmojiLikeNotice(ctx context.Context, event *napcat.NoticeEvent)
```

字段选择建议：

1. `message_id`：使用 `event.MessageID.String()`。
2. `user_id`：优先使用 `event.UserID.String()`，为空时 fallback 到 `event.OperatorID.String()`。
3. `face_id`：优先使用顶层 `event.EmojiID`；如果为空，从 `event.Likes` 中取 `EmojiId`。

如果 `Likes` 里有多个 emoji id，建议同一个 notice 内去重后逐个插入：

```text
message_id = event.MessageID
user_id = event.UserID or event.OperatorID
face_id = each unique emoji id
```

缺少 `message_id`、`user_id` 或 `face_id` 时只记录日志并跳过，不要让 notice goroutine panic。

### 与现有 notice 响应的关系

当前 `HandleNotice` 对 `TargetID == SelfID` 会发送固定文本 `灰色中分已然绽放`。`group_msg_emoji_like` 是数据记录事件，不应触发这条回复。

推荐结构：

```go
func (s *Service) HandleNotice(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.NoticeEvent) {
    if event == nil {
        return
    }

    if event.NoticeType == napcat.EventNoticeTypeGroupMsgEmojiLike {
        s.handleGroupMsgEmojiLikeNotice(ctx, event)
        return
    }

    // 保留现有 TargetID/SelfID 逻辑
}
```

## 测试建议

建议补以下测试：

1. `faceIDsFromSegments`：提取 face id、跳过空 id、同消息去重。
2. `ParseInboundMessage`：解析 `notice_type = "group_msg_emoji_like"`，覆盖顶层 `emoji_id` 和 `likes[].emoji_id` 两种形态。
3. `HandleNotice`：表情回应 notice 不触发现有 `灰色中分已然绽放` 回复逻辑。
4. `Store.UpsertFace`：重复 upsert 不报错。
5. `Store.SaveEmojiLike`：先 upsert face，再插入 emoji_like。

当前仓库没有专门的 DB 测试基础设施，Store 层测试可以先不做或使用临时 PostgreSQL；纯解析和 handler 分支测试更容易先落地。

## 验证命令

生成表和代码后建议执行：

```bash
sh create_njk_tables.sh
sh generate_gorm.sh
go test ./...
```

按仓库现有约定，完整本地验证再执行：

```bash
sh run_ws_server.sh
```

服务能成功启动即可，真实 NapCat notice 联调留给人工验证。

## 实施顺序

1. 修改 `sql/create_njk_tables.sql`，追加 `face` 和 `emoji_like`。
2. 执行 `sh create_njk_tables.sh`。
3. 新增 gorm/gen 生成器和 `generate_gorm.sh`。
4. 执行 `sh generate_gorm.sh`，确认生成 `Face`、`EmojiLike` 及 query。
5. 在 `Store` 增加 `UpsertFace` 和 `SaveEmojiLike`。
6. 在 `HandleGroupMessage` 白名单/黑名单过滤后保存 face segment 字典。
7. 在 `NoticeEvent` 增加顶层 `EmojiID`，并在 `HandleNotice` 早分支处理 `group_msg_emoji_like`。
8. 补测试并执行 `go test ./...`。
9. 执行 `sh run_ws_server.sh` 做启动验证。
