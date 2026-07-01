package bot

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"njk_go/internal/napcat"
)

func (s *Service) handleImageToFileCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	count, err := strconv.Atoi(match.Groups[1])
	if err != nil || count <= 0 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	history, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}

	files := imageToFileItemsFromMessages(history)
	if len(files) == 0 {
		return simpleOutbound(groupID, "最近消息里没有图片"), nil
	}
	return fileOutbound(groupID, files), nil
}

func imageToFileItemsFromMessages(messages []StoredMessage) []outboundFile {
	files := make([]outboundFile, 0)
	for _, message := range messages {
		var segments []napcat.MessageSegment
		if err := json.Unmarshal([]byte(message.RawJSON), &segments); err != nil {
			continue
		}
		for _, segment := range segments {
			if segment.Type != napcat.SegmentTypeImage {
				continue
			}
			url := strings.TrimSpace(segment.Data.URL)
			if url == "" {
				continue
			}
			files = append(files, outboundFile{
				URL:      url,
				FileName: strings.TrimSpace(segment.Data.File),
			})
		}
	}
	return files
}
