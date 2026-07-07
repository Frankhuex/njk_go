package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"njk_go/internal/napcat"
)

func (s *Service) systemPrompt(key commandKey) (string, error) {
	command := s.commandByKey(key)
	if command == nil {
		return "", fmt.Errorf("%s command not configured", key)
	}
	return command.Command.SystemPrompt, nil
}

func simpleOutbound(groupID string, message string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, Message: message, ShouldSave: false}
}

func imageOutbound(groupID string, imageURLs []string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeImage, ShouldSave: false}
}

func fileOutbound(groupID string, imageURLs []string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeFile, ShouldSave: false}
}

func segmentsOutbound(groupID string, segments []napcat.MessageSegment) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, Segments: segments, ShouldSave: false}
}

func insufficientHistory(groupID string) *pendingOutbound {
	return simpleOutbound(groupID, "历史消息不足")
}

func savedReplyOutbound(groupID string, replyMessageID string, message string) *pendingOutbound {
	return &pendingOutbound{
		GroupID:    groupID,
		Message:    fmt.Sprintf("[CQ:reply,id=%s]%s", replyMessageID, message),
		ShouldSave: true,
	}
}

func containsExact(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func randomRange(rng *rand.Rand, left int, right int) int {
	if right <= left {
		return left
	}
	return left + rng.Intn(right-left+1)
}

func sleepRandomMillis(ctx context.Context, rng *rand.Rand, left int, right int) error {
	delay := time.Duration(randomRange(rng, left, right)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func SleepRandomMillis(ctx context.Context, rng *rand.Rand, left int, right int) error {
	return sleepRandomMillis(ctx, rng, left, right)
}

func (s *Service) Random() *rand.Rand {
	if s == nil {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rng
}

func (s *Service) DownloadImage(ctx context.Context, sourceURL string) ([]byte, error) {
	if s == nil || s.imageService == nil {
		return nil, fmt.Errorf("image service not available")
	}
	return s.imageService.download(ctx, sourceURL)
}

func startOfReport(dayNum int) time.Time {
	now := time.Now()
	todayFive := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
	return todayFive.AddDate(0, 0, -dayNum)
}

func StructToKeyValue(v interface{}) (string, error) {
	// 1. 先序列化为 JSON 字节
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	// 2. 反序列化为 Map，此时键值对已经根据 JSON 标签对应好了
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}

	// 3. 遍历 Map 拼接字符串
	var pairs []string
	typeKey := "type"
	if typeVal, exists := m[typeKey]; exists {
		pairs = append(pairs, fmt.Sprintf("%s=%v", typeKey, typeVal))
	}
	for k, v := range m {
		if k == typeKey {
			continue
		}
		// fmt.Sprintf("%v") 可以把基础类型（数字、布尔、字符串）直接转为没有引号的文本
		// 注意：如果 val 是复杂类型（如切片或嵌套结构体），需要视具体需求特殊处理
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}

	// 4. 用逗号连接
	return strings.Join(pairs, ","), nil
}
