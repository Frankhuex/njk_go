package bot

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
)

const duplicateImageThreshold = 5

type ImageService struct {
	store      *Store
	httpClient *http.Client
}

func NewImageService(store *Store) *ImageService {
	return &ImageService{
		store: store,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *ImageService) SaveAndCheckDuplicate(ctx context.Context, groupID string, imageURL string, messageID string) (*DuplicateImage, error) {
	data, err := s.download(ctx, imageURL)
	if err != nil {
		return nil, err
	}
	hash, err := calculatePHash(data)
	if err != nil {
		return nil, err
	}

	record, err := s.store.SaveImage(ctx, messageID, hash)
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

	candidates, err := s.store.GroupImageCandidates(ctx, groupID, record.ID)
	if err != nil {
		return nil, err
	}

	target, err := decodeHash(hash)
	if err != nil {
		return nil, err
	}

	var duplicates []StoredImage
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

func (s *ImageService) EnsureEmojiWhitelist(ctx context.Context, groupID string, imageURL string) error {
	data, err := s.download(ctx, imageURL)
	if err != nil {
		return err
	}
	hash, err := calculatePHash(data)
	if err != nil {
		return err
	}
	candidates, err := s.store.GroupImageCandidates(ctx, groupID, 0)
	if err != nil {
		return err
	}
	target, err := decodeHash(hash)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		source, err := decodeHash(candidate.ImageHash)
		if err != nil {
			continue
		}
		if hammingDistance(target, source) <= duplicateImageThreshold {
			return s.store.AddWhitelistHash(ctx, hash)
		}
	}
	return nil
}

func (s *ImageService) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
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
