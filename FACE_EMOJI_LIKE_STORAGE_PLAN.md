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
3. `EmojiLike` 当前字段是 `EmojiID string`，JSON tag 为 `emoji_id`。
4. `NoticeEvent` 不应新增顶层 `EmojiID` 字段；`group_msg_emoji_like` 必须通过遍历 `NoticeEvent.Likes` 读取 `emoji_id`。
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

`internal/napcat/types.go` 已有 `Likes []EmojiLike`，不要给 `NoticeEvent` 新增顶层 `EmojiID` 字段。`group_msg_emoji_like` 只能遍历 `event.Likes` 读取 `EmojiLike.EmojiID`。

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
3. `face_id`：遍历 `event.Likes`，从每个 `EmojiLike.EmojiID` 读取。

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
2. `ParseInboundMessage`：解析 `notice_type = "group_msg_emoji_like"`，覆盖 `likes[].emoji_id`。
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
7. 保持 `NoticeEvent` 只使用 `Likes []EmojiLike`，并在 `HandleNotice` 早分支处理 `group_msg_emoji_like`。
8. 补测试并执行 `go test ./...`。
9. 执行 `sh run_ws_server.sh` 做启动验证。

## `.getfaceid` 指令方案

### 需求语义

新增指令：

```text
.getfaceid n
```

`n` 是整数，表示本群最近 `n` 条已保存消息。`.getfaceid` 和 `n` 之间可以有 0 个或任意多个空格，例如 `.getfaceid10` 和 `.getfaceid 10` 都应匹配。

执行流程：

1. 取本群前 `n` 条已保存消息。
2. 对每条消息，按之前 `.face` 已定义的方式解析 `message.raw_json`，取出消息 segments 中 `type = "face"` 的 `data.id`。
3. 对同一批消息，用 `message_id` 查询对应 `emoji_like` 记录，可以使用 `INNER JOIN`。
4. 最终输出按 message 分组，一条消息最多输出两行：segments 中的 face id 单独一行，`emoji_like.face_id` 单独一行。
5. 每一行内部的 face id 都递增排序，并用中文全角逗号 `，` 连接。
6. 同一条消息如果 segments 和 `emoji_like` 都没有 face id，则输出 0 行。
7. 作为普通文本消息发出。

输出建议带上来源前缀，避免两类 id 混在一起无法分辨，例如：

```text
消息123 segment：1，2，10
消息123 like：66，77
消息124 like：5
```

如果不希望带前缀，也至少要保持“segment 一行、emoji_like 一行”的固定顺序。但推荐带 `message_id` 和来源标签，方便用户知道每行对应哪条消息。

### 命令注册

在 `internal/bot/prompts.go` 新增 command key：

```go
commandGetFaceID commandKey = "get_face_id"
```

在 `commandDefs` 中新增正则：

```go
{
    Key:     commandGetFaceID,
    Pattern: `^ *\.getfaceid *(\d+) *$`,
}
```

这个正则与现有 `.face` / `.faceid` 不冲突。`matchCommand` 会从后往前匹配，但 `.getfaceid` 前缀独立，不依赖顺序兜底。

建议更新 `helpText`，增加一行：

```text
.getfaceid 后面接数字，表示读取本群最近消息收到过的表情回应id
```

### Handler 接入

在 `internal/bot/commands.go` 的 `buildCommandHandler` 增加分支：

```go
case commandGetFaceID:
    return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
        return s.handleGetFaceIDCommand(ctx, event.GroupID.String(), match)
    }
```

新增文件建议：

```text
internal/bot/command_getfaceid.go
```

handler 逻辑：

```go
func (s *Service) handleGetFaceIDCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
    // 1. 校验捕获组
    // 2. strconv.Atoi(match.Groups[1])
    // 3. n <= 0 返回 simpleOutbound(groupID, "参数错误")
    // 4. 调用 s.store.RecentFaceIDRows(ctx, groupID, n)
    // 5. 没有结果时建议返回 simpleOutbound(groupID, "没有查到")，避免发送空消息
    // 6. formatGetFaceIDRows(rows) 后 simpleOutbound(groupID, text)
}
```

该命令是显式命令，按当前 `HandleGroupMessage` 逻辑不会保存触发命令本身，因此“前 `n` 条消息”自然不包含当前 `.getfaceid` 指令消息。

### Store 查询设计

在 `internal/bot/store.go` 新增：

```go
func (s *Store) RecentFaceIDRows(ctx context.Context, groupID string, limit int) ([]GetFaceIDMessageRow, error)
```

建议定义内部结构：

```go
type GetFaceIDMessageRow struct {
    MessageID       string
    SegmentFaceIDs  []string
    EmojiLikeFaceIDs []string
}
```

查询建议分两步，避免把 `raw_json` 解析和 `emoji_like` 聚合揉成一条复杂 SQL：

1. 先取最近 `n` 条消息，拿到 `message_id`、`time`、`raw_json`。
2. 再用这些 `message_id` 查 `emoji_like`，按 `message_id` 分组聚合。

最近消息查询可以复用 `RecentMessages`，但它会 reverse 成时间正序；这里建议新方法自己查，保留最近消息的稳定顺序：

```sql
SELECT m.message_id,
       m."time",
       COALESCE(m.raw_json::text, '') AS raw_json
FROM message AS m
WHERE m.group_id = ?
ORDER BY m."time" DESC
LIMIT ?;
```

`emoji_like` 查询推荐 SQL 形态：

```sql
SELECT recent.message_id,
       el.face_id
FROM emoji_like AS el
INNER JOIN (
    SELECT m.message_id, m."time"
    FROM message AS m
    WHERE m.group_id = ?
    ORDER BY m."time" DESC
    LIMIT ?
) AS recent ON recent.message_id = el.message_id
ORDER BY
    recent."time" DESC,
    CASE WHEN el.face_id ~ '^\d+$' THEN el.face_id::bigint END ASC NULLS LAST,
    el.face_id ASC;
```

说明：

1. 子查询先限定“本群最近 `n` 条消息”，避免全表 join 后再 limit 导致语义错误。
2. `INNER JOIN` 符合需求，只返回存在 `emoji_like` 的消息；没有 like 的消息仍会从第一步最近消息查询中保留，用于输出 segment face id。
3. `face_id` 是 `VARCHAR(30)`，但当前 face id 语义是数字。用正则保护后转 `bigint`，可实现数字递增排序，避免字符串排序出现 `10` 排在 `2` 前面。
4. `emoji_like` 查询不使用 `DISTINCT`，因为需求说“所有 `emoji_like` 的 `face_id` 取出”。如果同一个 face_id 出现多次，应保留重复值。
5. segments 中的 face id 来自一条消息内部，建议本地排序。是否去重沿用 `faceIDsFromSegments` 当前语义；如果要严格保留同一条消息里重复 face segment，应新增一个不去重的 extractor。

GORM 写法可以使用 `Raw`，更直观：

```go
type emojiLikeFaceIDRow struct {
    MessageID string `gorm:"column:message_id"`
    FaceID    string `gorm:"column:face_id"`
}

var rows []emojiLikeFaceIDRow
err := s.db.WithContext(ctx).Raw(`
    SELECT recent.message_id, el.face_id
    FROM emoji_like AS el
    INNER JOIN (
        SELECT m.message_id, m."time"
        FROM message AS m
        WHERE m.group_id = ?
        ORDER BY m."time" DESC
        LIMIT ?
    ) AS recent ON recent.message_id = el.message_id
    ORDER BY
        recent."time" DESC,
        CASE WHEN el.face_id ~ '^\d+$' THEN el.face_id::bigint END ASC NULLS LAST,
        el.face_id ASC
`, groupID, limit).Scan(&rows).Error
```

segments 中的 face id 处理：

```go
segmentFaceIDs, err := extractFaceIDsFromRawJSON(rawJSON)
if err != nil {
    segmentFaceIDs = nil
}
sortFaceIDs(segmentFaceIDs)
```

建议抽出数字友好的排序函数，两类来源共用：

```go
func sortFaceIDs(faceIDs []string)
```

排序规则与 SQL 一致：能解析成整数的按整数递增，无法解析成整数的排在数字后面并按字符串递增。

### 输出格式设计

建议新增格式化 helper：

```go
func formatGetFaceIDRows(rows []GetFaceIDMessageRow) string
```

格式化规则：

1. 遍历最近消息顺序。
2. 如果 `SegmentFaceIDs` 非空，输出一行：`消息<message_id> segment：<ids>`。
3. 如果 `EmojiLikeFaceIDs` 非空，输出一行：`消息<message_id> like：<ids>`。
4. `<ids>` 使用 `strings.Join(ids, "，")`。
5. 全部为空时返回空字符串，由 handler 转成 `没有查到`。

示例：

```text
消息1003 segment：1，2，10
消息1003 like：5，66
消息1002 segment：14
消息1001 like：77，77
```

这里 `消息1001 like：77，77` 表示 `emoji_like` 表里有两条记录，按“不去重”语义保留。

### 测试建议

建议补以下测试：

1. `TestMatchCommandSupportsGetFaceIDWithOptionalSpace`：`.getfaceid12` 和 `.getfaceid 12` 都匹配 `commandGetFaceID`。
2. `TestMatchCommandRejectsInvalidGetFaceID`：`.getfaceid abc`、`.getfaceid` 不匹配。
3. `TestHandleGetFaceIDCommandRejectsNonPositiveLimit`：`.getfaceid 0` 返回 `参数错误`。
4. `TestFormatGetFaceIDRowsGroupsByMessageAndSource`：同一消息同时有 segment 和 like 时输出两行，且 segment 行在 like 行前。
5. `TestSortFaceIDsNumericOrder`：`[]string{"10", "2", "1"}` 排成 `[]string{"1", "2", "10"}`。
6. 如果引入 DB 测试，覆盖 `RecentFaceIDRows`：构造多条不同时间消息、raw_json face segment 与 emoji_like，确认只取最近 `n` 条消息内的记录，按 message 分组，行内数字递增排序，`emoji_like` 重复 face_id 不去重。

当前 `Service` 直接持有 `*Store`，handler 单测如果不接真实 DB 会不方便 mock。最小改法是先覆盖命令匹配和纯格式化 helper，例如抽出：

```go
func formatGetFaceIDRows(rows []GetFaceIDMessageRow) string {
    // join per-message, per-source lines with "\n"
}
```

Store 层行为可先通过 SQL 评审和人工联调验证，后续再补 PostgreSQL 集成测试。

### 验证命令

实现后建议执行：

```bash
go test ./internal/bot ./internal/napcat ./internal/model ./internal/query
```

如果现有 `.faceid` 测试仍未修复，`go test ./...` 可能继续失败；需要区分本次 `.getfaceid` 改动与已有失败。
