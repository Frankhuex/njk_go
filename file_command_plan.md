# .file Command Implementation Plan

## Goal

Add a new group command:

```text
.file n
.filen
```

It reads the previous `n` persisted messages in the current group, extracts saved image/GIF URLs from those messages, and sends them back as NapCat `file` segments instead of `image` segments.

The count may appear directly after `file` or after spaces, matching existing command style such as `.json12`, `.face12`, and `.对称左5`.

## Current Findings

- Commands are registered in `internal/bot/prompts.go` through `commandKey` constants and `commandDefs` regex entries.
- Command dispatch is centralized in `internal/bot/commands.go` through `buildCommandHandler`.
- Image history lookup already exists in `Store.RecentMessageImages(ctx, groupID, limit)` in `internal/bot/store.go`.
- `.对称*` commands already use `RecentMessageImages` to retrieve images from the previous `n` persisted messages.
- `internal/napcat/types.go` already defines `napcat.SegmentTypeFile = "file"`.
- `internal/bot/service_ingress.go` already has `multiSendGroupImages(ctx, conn, groupID, imgURLs, segmentType)` and the comment says `segmentType` must be `image` or `file`.
- The current unified send loop always calls `multiSendGroupImages(..., napcat.SegmentTypeImage)` for `pendingOutbound.ImageURLs`, so `.file` needs a way to carry `SegmentTypeFile` through `pendingOutbound`.

## Data Source

Use `Store.RecentMessageImages(ctx, groupID, count)` as the initial implementation data source.

This means `.file n` will use URLs persisted in the `image` table, not re-parse `message.raw_json` directly.

Important edge cases:

- `image.url` is nullable, so nil or blank URLs must be skipped.
- The command can only resend images/GIFs that were successfully saved to the `image` table during ingestion.
- If an image segment existed in `raw_json` but `SaveAndCheckDuplicate` failed during ingestion, `.file` will not see it through `RecentMessageImages`.
- Command trigger messages are not saved before command execution, so `.file n` naturally reads messages before the `.file` command itself.

## Ordering Note

`RecentMessageImages` currently selects images whose `message_id` is in the recent message subquery, but the outer image query has no explicit `ORDER BY`.

For `.file n`, deterministic ordering is more user-visible than for some image-processing commands. Recommended minimal improvement:

```go
Order("i.message_id ASC, i.id ASC")
```

However, `message_id` ordering is not equivalent to message time ordering. Better implementation if strict chronological order matters:

```sql
JOIN message m ON i.message_id = m.message_id
WHERE i.message_id IN (?)
ORDER BY m.time ASC, i.id ASC
```

Recommended approach:

- Update `RecentMessageImages` to join `message` and order by `m.time ASC, i.id ASC`.
- This also improves `.对称*` output determinism.
- No schema change is required.

## Proposed Code Changes

### 1. Add Command Key

File: `internal/bot/prompts.go`

Add near other history/debug commands:

```go
commandFile commandKey = "file"
```

### 2. Add Help Text

File: `internal/bot/prompts.go`

Add one help line:

```text
.file 后面接数字，表示把本群最近消息里的图片/动图作为文件发出
```

### 3. Add Regex

File: `internal/bot/prompts.go`

Add to `commandDefs`:

```go
{
    Key:     commandFile,
    Pattern: `^ *\.file *(\d+) *$`,
},
```

This supports both `.file12` and `.file 12`.

### 4. Carry Segment Type Through pendingOutbound

File: `internal/bot/state.go`

Add a field:

```go
ImageSegmentType napcat.SegmentType
```

This requires importing `njk_go/internal/napcat` in `state.go`.

Alternative is to add a separate `FileURLs []string`, but `ImageSegmentType` is smaller and reuses the existing image URL send path.

### 5. Add Outbound Helper

File: `internal/bot/helpers.go`

Keep existing `imageOutbound` behavior as image segments:

```go
func imageOutbound(groupID string, imageURLs []string) *pendingOutbound {
    return &pendingOutbound{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeImage, ShouldSave: false}
}
```

Add:

```go
func fileOutbound(groupID string, fileURLs []string) *pendingOutbound {
    return &pendingOutbound{GroupID: groupID, ImageURLs: fileURLs, ImageSegmentType: napcat.SegmentTypeFile, ShouldSave: false}
}
```

### 6. Update Unified Send Loop

File: `internal/bot/service_ingress.go`

Current code:

```go
if len(response.ImageURLs) > 0 {
    if err := s.multiSendGroupImages(ctx, conn, response.GroupID, response.ImageURLs, napcat.SegmentTypeImage); err != nil {
        ...
    }
}
```

Proposed behavior:

```go
segmentType := response.ImageSegmentType
if segmentType == "" {
    segmentType = napcat.SegmentTypeImage
}
if len(response.ImageURLs) > 0 {
    if err := s.multiSendGroupImages(ctx, conn, response.GroupID, response.ImageURLs, segmentType); err != nil {
        ...
    }
}
```

This preserves all existing image command behavior while allowing `.file` to send file segments.

### 7. Add Command Handler

Recommended new file:

```text
internal/bot/command_file.go
```

Handler outline:

```go
func (s *Service) handleFileCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
    if len(match.Groups) < 2 {
        return simpleOutbound(groupID, "参数错误"), nil
    }

    count, err := strconv.Atoi(match.Groups[1])
    if err != nil || count <= 0 {
        return simpleOutbound(groupID, "参数错误"), nil
    }

    images, err := s.store.RecentMessageImages(ctx, groupID, count)
    if err != nil {
        return nil, err
    }

    urls := imageURLsFromRecords(images)
    if len(urls) == 0 {
        return simpleOutbound(groupID, "最近消息里没有图片"), nil
    }

    return fileOutbound(groupID, urls), nil
}
```

Helper outline:

```go
func imageURLsFromRecords(images []model.Image) []string {
    urls := make([]string, 0, len(images))
    for _, item := range images {
        if item.URL == nil || strings.TrimSpace(*item.URL) == "" {
            continue
        }
        urls = append(urls, strings.TrimSpace(*item.URL))
    }
    return urls
}
```

This helper is easy to unit test without database setup.

### 8. Add Dispatch Branch

File: `internal/bot/commands.go`

Add:

```go
case commandFile:
    return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
        return s.handleFileCommand(ctx, event.GroupID.String(), match)
    }
```

## Send Semantics

`.file` should send one `send_group_msg` action with multiple segments:

```json
[
 {
  "type": "file",
  "data": {
   "file": "<image-or-gif-url>"
  }
 }
]
```

Because it uses `multiSendGroupImages`, the action still goes through `send_group_msg`, enters `pendingQueue` with `ShouldSave=false`, and will not save bot output in `message`.

## Save Behavior

Expected behavior:

- The `.file` trigger message itself is not saved by the current command flow.
- The bot's file response is not saved because the outbound uses `ShouldSave=false`.
- The command reads only already persisted image metadata.

## Tests To Add

### Command Matching

File: `internal/bot/service_test.go` or `internal/bot/command_symmetric_test.go`

Add cases:

- `.file12` matches `commandFile` and captures `12`.
- `.file 12` matches `commandFile` and captures `12`.
- `.file abc` does not match.

### URL Extraction Helper

File: `internal/bot/service_test.go` or new `internal/bot/command_file_test.go`

Add cases:

- Nil URL is skipped.
- Blank URL is skipped.
- Valid URL is retained and trimmed.
- Input order is preserved.

### Send Segment Type

Existing WebSocket-style tests can be expanded, but the minimal test should avoid net pipe complexity:

- Test `fileOutbound` returns `ImageSegmentType == napcat.SegmentTypeFile`.
- Test `imageOutbound` returns `ImageSegmentType == napcat.SegmentTypeImage` or default behavior remains image.

If adding a higher-level send test later, inspect the generated `send_group_msg` payload and assert segment `type == "file"`.

## Validation Plan

After implementation:

1. Run `gofmt` on modified Go files.
2. Run target package tests:

```bash
go test -v -gcflags="all=-l -N" ./internal/bot
```

3. Run full tests and compile:

```bash
go test ./...
go build ./...
```

No service startup is needed unless explicitly requested.

## Open Risks

- NapCat may require different fields for file segments depending on whether `send_group_msg` accepts URL-backed file sending in the active adapter version. The current project types and helper support `type=file` with `data.file`, so implement against that existing contract first.
- `RecentMessageImages` depends on image-table rows; raw image segments that failed image hashing/download during ingestion will not be included.
- Sending many files in one group message may hit NapCat or platform limits. Keep the first implementation simple unless real failures show a need for splitting or a max count.
