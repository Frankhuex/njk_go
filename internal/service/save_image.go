package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"strings"
	"time"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/napcat"

	"github.com/corona10/goimagehash"
	_ "golang.org/x/image/webp"
)

const duplicateImageThreshold = 5

func (s *Service) SaveAndCheckDuplicate(ctx context.Context, groupID string, imageURL string, messageID string) (*DuplicateImage, error) {
	data, err := s.DownloadImageBytes(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	hash, err := calculatePHash(data)
	if err != nil {
		return nil, err
	}

	record, err := s.store.SaveImage(ctx, messageID, hash, imageURL)
	if err != nil {
		return nil, err
	}

	whitelisted, err := s.store.IsHashWhitelisted(ctx, hash)
	if err != nil {
		return nil, err
	}
	if whitelisted {
		return nil, nil
	}

	candidates, err := s.store.GroupImageCandidates(ctx, groupID, record.ID, messageID)
	if err != nil {
		return nil, err
	}

	target, err := decodeHash(hash)
	if err != nil {
		return nil, err
	}

	var duplicates []pgstore.StoredImage
	for _, candidate := range candidates {
		source, err := decodeHash(candidate.ImageHash)
		if err != nil {
			continue
		}
		if hammingDistance(target, source) <= duplicateImageThreshold {
			duplicates = append(duplicates, candidate)
		}
	}
	if len(duplicates) == 0 {
		return nil, nil
	}

	earliest := duplicates[0]
	for _, item := range duplicates[1:] {
		if item.Time.Before(earliest.Time) {
			earliest = item
		}
	}

	name := earliest.Card
	if name == "" {
		name = earliest.Nickname
	}
	if name == "" {
		name = "Unknown User"
	}

	return &DuplicateImage{
		Count:      len(duplicates),
		MessageID:  earliest.MessageID,
		SenderName: name,
		SentAt:     earliest.Time,
	}, nil
}

func (s *Service) EnsureEmojiWhitelist(ctx context.Context, groupID string, imageURL string) error {
	data, err := s.DownloadImageBytes(ctx, imageURL)
	if err != nil {
		log.Printf("【下载图片失败】group=%s url=%s err=%v", groupID, imageURL, err)
		return err
	}
	hash, err := calculatePHash(data)
	if err != nil {
		log.Printf("【计算图片哈希失败】group=%s url=%s err=%v", groupID, imageURL, err)
		return err
	}
	candidates, err := s.store.GroupImageCandidates(ctx, groupID, 0, "")
	if err != nil {
		log.Printf("【查询图片候选失败】group=%s err=%v", groupID, err)
		return err
	}
	target, err := decodeHash(hash)
	if err != nil {
		log.Printf("【解码图片哈希失败】group=%s url=%s err=%v", groupID, imageURL, err)
		return err
	}
	for _, candidate := range candidates {
		source, err := decodeHash(candidate.ImageHash)
		if err != nil {
			log.Printf("【解码图片候选哈希失败】group=%s url=%s err=%v", groupID, candidate.ImageHash, err)
			continue
		}
		if hammingDistance(target, source) <= duplicateImageThreshold {
			return s.store.AddWhitelistHash(ctx, hash)
		}
	}
	return nil
}

func (s *Service) DownloadImageBytes(ctx context.Context, url string) ([]byte, error) {
	if s == nil || s.httpClient == nil {
		return nil, fmt.Errorf("http client not available")
	}
	return s.httpClient.DownloadBytes(ctx, url)
}

func calculatePHash(data []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%016x", hash.GetHash()), nil
}

func decodeHash(hash string) ([]byte, error) {
	hash = normalizeStoredHash(hash)
	return hex.DecodeString(hash)
}

func normalizeStoredHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) >= 2 && hash[1] == ':' {
		return hash[2:]
	}
	return hash
}

func hammingDistance(left []byte, right []byte) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	distance := 0
	for i := 0; i < limit; i++ {
		distance += bitsCount(left[i] ^ right[i])
	}
	return distance
}

func bitsCount(v byte) int {
	count := 0
	for v > 0 {
		count += int(v & 1)
		v >>= 1
	}
	return count
}

type DuplicateImage struct {
	Count      int
	MessageID  string
	SenderName string
	SentAt     time.Time
}

func isEmojiImage(segment napcat.MessageSegment) bool {
	data := segment.Data
	return data.EmojiID != "" || data.EmojiPackageID != 0 || data.Key != "" || data.SubType == 1 || strings.Contains(data.Summary, "动画表情")
}
