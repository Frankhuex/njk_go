package service

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func (s *Service) completeWithMultimodalFallback(ctx context.Context, groupID string, systemPrompt string, text string, imageURLs []string, temperature *float64) (string, error) {
	if s == nil || s.aiClient == nil {
		return "", fmt.Errorf("ai client not configured")
	}

	text = strings.TrimSpace(text)
	filteredImageURLs := make([]string, 0, len(imageURLs))
	for _, imageURL := range imageURLs {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		filteredImageURLs = append(filteredImageURLs, imageURL)
	}

	if len(filteredImageURLs) == 0 {
		return s.aiClient.Complete(ctx, systemPrompt, text, temperature)
	}

	result, err := s.aiClient.CompleteMultimodal(ctx, systemPrompt, text, filteredImageURLs, temperature)
	if err == nil {
		return result, nil
	}

	log.Printf("【多模态降级到最新图】group=%s images=%d err=%v", groupID, len(filteredImageURLs), err)
	latestOnly := []string{filteredImageURLs[len(filteredImageURLs)-1]}
	result, latestErr := s.aiClient.CompleteMultimodal(ctx, systemPrompt, text, latestOnly, temperature)
	if latestErr == nil {
		return result, nil
	}

	log.Printf("【多模态回退单模态】group=%s latest_image=1 err=%v", groupID, latestErr)
	fallbackResult, fallbackErr := s.aiClient.Complete(ctx, systemPrompt, text, temperature)
	if fallbackErr != nil {
		return "", fmt.Errorf("multimodal failed: %v; latest-image multimodal failed: %v; fallback failed: %w", err, latestErr, fallbackErr)
	}
	return fallbackResult, nil
}
