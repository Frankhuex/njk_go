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
	"math"
	"net/http"
	"sort"
	"time"
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
		Count:       len(duplicates),
		MessageID:   earliest.MessageID,
		SenderName:  name,
		SentAt:      earliest.Time,
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
	return phashImage(img), nil
}

func phashImage(img image.Image) string {
	gray := resizeGray(img, 32, 32)
	dct := dct2(gray)
	values := make([]float64, 0, 63)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x == 0 && y == 0 {
				continue
			}
			values = append(values, dct[y][x])
		}
	}
	median := median(values)
	bits := make([]byte, 8)
	index := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			var bit byte
			if dct[y][x] > median {
				bit = 1
			}
			byteIndex := index / 8
			bits[byteIndex] = (bits[byteIndex] << 1) | bit
			index++
		}
	}
	return hex.EncodeToString(bits)
}

func resizeGray(img image.Image, width int, height int) [][]float64 {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	result := make([][]float64, height)
	for y := 0; y < height; y++ {
		row := make([]float64, width)
		srcY := bounds.Min.Y + y*srcH/height
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + x*srcW/width
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			row[x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
		}
		result[y] = row
	}
	return result
}

func dct2(input [][]float64) [][]float64 {
	size := len(input)
	output := make([][]float64, size)
	for v := 0; v < size; v++ {
		row := make([]float64, size)
		cv := coefficient(v, size)
		for u := 0; u < size; u++ {
			cu := coefficient(u, size)
			sum := 0.0
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					sum += input[y][x] *
						math.Cos((float64(2*x+1)*float64(u)*math.Pi)/(2*float64(size))) *
						math.Cos((float64(2*y+1)*float64(v)*math.Pi)/(2*float64(size)))
				}
			}
			row[u] = cu * cv * sum
		}
		output[v] = row
	}
	return output
}

func coefficient(index int, size int) float64 {
	if index == 0 {
		return math.Sqrt(1.0 / float64(size))
	}
	return math.Sqrt(2.0 / float64(size))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	mid := len(copyValues) / 2
	if len(copyValues)%2 == 0 {
		return (copyValues[mid-1] + copyValues[mid]) / 2
	}
	return copyValues[mid]
}

func decodeHash(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
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
