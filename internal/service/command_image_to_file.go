package service

import (
	"context"
	"strconv"
	"strings"

	"njk_go/internal/dal/model"
)

func (s *Service) handleImageToFileCommand(ctx context.Context, groupID string, match CommandMatch) (*pendingOutbound, error) {
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

	fileURLs := imageToFileSourceURLsFromRecords(images)
	if len(fileURLs) == 0 {
		return simpleOutbound(groupID, "最近消息里没有图片"), nil
	}
	return fileOutbound(groupID, fileURLs), nil
}

func imageToFileSourceURLsFromRecords(images []model.Image) []string {
	urls := make([]string, 0, len(images))
	for _, image := range images {
		if image.URL == nil {
			continue
		}
		url := strings.TrimSpace(*image.URL)
		if url == "" {
			continue
		}
		urls = append(urls, url)
	}
	return urls
}
