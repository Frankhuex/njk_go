package service

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/dal/model"
)

var imagePromptBracketPattern = regexp.MustCompile(`\[[^\[\]]*\]`)

const defaultImageOnlyPrompt = "基于参考图生成一张风格协调、细节清晰的新图片"

func (s *Service) handleGenerateImageCommand(ctx context.Context, groupID string, match CommandMatch) (*OutboundAction, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	if s == nil || s.store == nil || s.imageGenClient == nil {
		return simpleOutbound(groupID, "生图服务未配置"), nil
	}

	count, err := strconv.Atoi(match.Groups[1])
	if err != nil || count <= 0 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	log.Printf("【生图命令开始】group=%s n=%d", groupID, count)
	messages, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	images, err := s.store.RecentMessageImages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}

	prompt := buildImagePromptFromMessages(messages)
	imageURL := latestImageURL(images)
	if prompt == "" && imageURL == "" {
		return simpleOutbound(groupID, "最近消息里没有可用于生图的内容"), nil
	}
	if prompt == "" && imageURL != "" {
		prompt = defaultImageOnlyPrompt
	}
	if prompt != "" {
		log.Printf("【生图文本选中】group=%s n=%d text_len=%d", groupID, count, len(prompt))
	}
	if imageURL != "" {
		log.Printf("【生图参考图选中】group=%s image_url=%s", groupID, imageURL)
	}

	log.Printf("【生图请求开始】group=%s model=%s", groupID, s.cfg.ImageGenModelName)
	generatedURL, err := s.imageGenClient.Generate(ctx, prompt, imageURL)
	if err != nil {
		log.Printf("【生图请求失败】group=%s err=%v", groupID, err)
		return simpleOutbound(groupID, "生图失败，请稍后再试"), nil
	}
	log.Printf("【生图请求成功】group=%s output_url=%s", groupID, generatedURL)
	return imageOutbound(groupID, []string{generatedURL}), nil
}

func buildImagePromptFromMessages(messages []pgstore.StoredMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		raw := firstNonEmpty(message.RawMessage, message.Text)
		sanitized := sanitizeImagePromptText(raw)
		if sanitized != "" {
			parts = append(parts, sanitized)
		}
	}
	return strings.Join(parts, "\n")
}

func sanitizeImagePromptText(raw string) string {
	raw = imagePromptBracketPattern.ReplaceAllString(raw, " ")
	raw = strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	return raw
}

func latestImageURL(images []model.Image) string {
	for i := len(images) - 1; i >= 0; i-- {
		if images[i].URL == nil {
			continue
		}
		url := strings.TrimSpace(*images[i].URL)
		if url != "" {
			return url
		}
	}
	return ""
}
