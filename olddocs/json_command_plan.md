# .json Command Implementation Plan

## Goal

Add a new group command:

```text
.json n
.jsonn
```

It reads the previous `n` persisted messages in the current group and sends their `message.raw_json` values back to the group.

The count may appear directly after `json` or after spaces, matching existing command style such as `.face12` and `.face 12`.

## Current Findings

- Commands are registered in `internal/bot/prompts.go` through `commandKey` constants and `commandDefs` regex entries.
- Command dispatch is centralized in `internal/bot/commands.go` through `buildCommandHandler`.
- Historical messages are already available through `Store.RecentMessages(ctx, groupID, limit)` in `internal/bot/store.go`.
- `StoredMessage` already includes `RawJSON`, so no store schema change is needed.
- `RecentMessages` returns messages in chronological order after querying latest `n` rows and reversing them.
- Command trigger messages are not saved before command execution unless the command is `commandNJK`; therefore `.json n` will naturally read messages before the `.json` command itself.
- Existing `.face` already reads `RawJSON` from recent messages, but it attempts to unmarshal only segment arrays. `.json` should not do that because saved `raw_json` may be either a JSON array or a JSON string.

## Data Semantics

Observed and code-supported `raw_json` forms:

- Inbound group messages: JSON array of NapCat message segments, for example `[{"type":"text","data":{"text":"..."}}]`.
- Saved bot text replies: JSON string, because `saveSelfMessage` stores `json.Marshal(pending.Message)`.

The `.json` command should treat `RawJSON` as already-serialized JSON text and preserve its JSON type instead of converting everything to strings.

Recommended response payload format:

```json
[
  <raw_json_of_oldest_selected_message>,
  <raw_json_of_next_selected_message>
]
```

Reasons:

- The response remains valid JSON.
- Array-type raw JSON stays an array.
- String-type raw JSON stays a string.
- The output contains exactly the requested raw JSON values, without adding message metadata.

If debugging context is needed later, a separate command such as `.jsonv n` can include `message_id`, `time`, and sender metadata. The requested `.json n` should stay minimal.

## Proposed Code Changes

### 1. Register Command Key

File: `internal/bot/prompts.go`

Add:

```go
commandJSON commandKey = "json"
```

Place it near other history/debug commands, for example after `commandFace`.

### 2. Register Regex

File: `internal/bot/prompts.go`

Add to `commandDefs`:

```go
{
    Key:     commandJSON,
    Pattern: `^ *\.json *(\d+) *$`,
},
```

This supports both `.json12` and `.json 12`.

Potential conflict risk is low because no current command begins with `.json`.

### 3. Add Handler Dispatch

File: `internal/bot/commands.go`

Add a switch branch:

```go
case commandJSON:
    return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
        return s.handleJSONCommand(ctx, event.GroupID.String(), match)
    }
```

The handler should return `simpleOutbound`, so the bot response is not saved.

### 4. Add Command Handler File

Recommended new file:

```text
internal/bot/command_json.go
```

Handler outline:

```go
func (s *Service) handleJSONCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
    count, err := strconv.Atoi(match.Groups[1])
    if err != nil || count <= 0 {
        return simpleOutbound(groupID, "参数错误"), nil
    }

    history, err := s.store.RecentMessages(ctx, groupID, count)
    if err != nil {
        return nil, err
    }
    if len(history) == 0 {
        return insufficientHistory(groupID), nil
    }

    output, err := formatRawJSONMessages(history)
    if err != nil {
        return nil, err
    }
    return simpleOutbound(groupID, output), nil
}
```

Formatting helper outline:

```go
func formatRawJSONMessages(messages []StoredMessage) (string, error) {
    values := make([]json.RawMessage, 0, len(messages))
    for _, msg := range messages {
        raw := strings.TrimSpace(msg.RawJSON)
        if raw == "" {
            raw = "null"
        }
        if !json.Valid([]byte(raw)) {
            rawBytes, err := json.Marshal(raw)
            if err != nil {
                return "", err
            }
            values = append(values, rawBytes)
            continue
        }
        values = append(values, json.RawMessage(raw))
    }
    data, err := json.Marshal(values)
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

Notes:

- `json.RawMessage` preserves each stored value as JSON.
- `json.Valid` makes the helper robust if historical rows contain malformed text, although current schema is `JSONB` so valid JSON is expected.
- Compact JSON is preferable for group messages because pretty JSON can exceed message length faster.

### 5. Help Text

File: `internal/bot/prompts.go`

Add one help line:

```text
.json 后面接数字，返回本群最近消息的 raw_json
```

Suggested wording should mention that it reads persisted history, not necessarily every group event.

## Output Size Risk

This command can easily generate a very long group message, especially when images include long URLs or when `n` is large.

Options:

- Minimal implementation: no explicit cap, same as several existing history commands. This follows the requested semantics most closely but may fail when NapCat or QQ rejects an oversized message.
- Safer implementation: add a count cap, for example `n <= 20`, and return `太多啦，最多20条` when exceeded.
- More robust implementation: add output truncation or split into multiple messages, but this changes the current simple outbound model and is more invasive.

Recommended first implementation:

- Add a small count cap such as `20` only if oversized messages become a real issue.
- Otherwise keep the command simple and let `sendGroupText` handle the send path consistently with existing commands.

## Save Behavior

`.json` should return `simpleOutbound`, not `savedReplyOutbound`.

Expected behavior:

- The `.json` trigger message itself is not saved by the current command flow.
- The bot's JSON response is not saved.
- The command is a read-only diagnostic command over already persisted message history.

## Tests To Add

File: `internal/bot/service_test.go`

Add tests for command matching:

- `.json12` matches `commandJSON` and captures `12`.
- `.json 12` matches `commandJSON` and captures `12`.
- `.json abc` does not match.

Add tests for formatting helper:

- Array raw JSON remains an array element in the output JSON array.
- String raw JSON remains a string element in the output JSON array.
- Empty raw JSON becomes `null` if the helper chooses that fallback.

If adding a handler-level test, create a small fake store only if the current `Service` design makes it easy. Otherwise keep the test focused on `formatRawJSONMessages`, because `RecentMessages` already has no direct unit-test infrastructure in this repo.

## Implementation Order

1. Add `commandJSON` key.
2. Add `.json *(\d+)` regex to `commandDefs`.
3. Add `commandJSON` branch in `buildCommandHandler`.
4. Add `internal/bot/command_json.go` with handler and formatter.
5. Update `helpText`.
6. Add unit tests for matching and formatting.
7. Run `go test ./...`.
8. Optionally run `sh run_ws_server.sh` only when ready for full local verification.

## Open Decision

The only design choice worth confirming before coding is whether to enforce a maximum `n` or output length. My recommended default is to avoid a cap initially, because the user-facing requirement is explicitly to return the previous `n` raw JSON values.
