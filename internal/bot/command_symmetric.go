package bot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"njk_go/internal/model"
)

type imageKind int

const (
	imageKindUnknown imageKind = iota
	imageKindStatic
	imageKindGIF
)

const symmetricImageMaxConcurrency = 4

func (s *Service) handleSymmetricLeftCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	count, err := strconv.Atoi(match.Groups[1])
	if err != nil || count < 1 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	images, err := s.store.RecentMessageImages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return simpleOutbound(groupID, "最近消息里没有图片"), nil
	}

	outputURLs := make([]string, len(images))
	sem := make(chan struct{}, symmetricImageMaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for idx, item := range images {
		if item.URL == nil || strings.TrimSpace(*item.URL) == "" {
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(idx int, item model.Image) {
			defer wg.Done()
			defer func() { <-sem }()

			data, err := s.imageService.download(ctx, *item.URL)
			if err != nil {
				return
			}

			imageURL, err := s.makeSymmetricLeftImage(ctx, item, data, *item.URL)
			if err != nil {
				return
			}

			mu.Lock()
			outputURLs[idx] = imageURL
			mu.Unlock()
		}(idx, item)
	}
	wg.Wait()

	outputURLs = compactStringsInOrder(outputURLs)
	if len(outputURLs) == 0 {
		return simpleOutbound(groupID, "最近消息里没有可处理的图片"), nil
	}
	return imageOutbound(groupID, outputURLs), nil
}

func (s *Service) makeSymmetricLeftImage(ctx context.Context, item model.Image, data []byte, sourceURL string) (string, error) {
	kind := detectImageKind(sourceURL, data)
	baseName := symmetricFileBase(item.MessageID, item.ID)

	switch kind {
	case imageKindGIF:
		animated, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		result := makeSymmetricGIF(animated)
		fileName := baseName + ".gif"
		if err := s.imageStore.SaveGIF(result, fileName); err != nil {
			return "", err
		}
		return s.imageStore.ReadImage(fileName)
	case imageKindStatic:
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		result := makeSymmetricStatic(img)
		fileName := baseName + ".png"
		if err := s.imageStore.SavePNG(result, fileName); err != nil {
			return "", err
		}
		return s.imageStore.ReadImage(fileName)
	default:
		return "", fmt.Errorf("unsupported image type")
	}
}

func detectImageKind(sourceURL string, data []byte) imageKind {
	ext := normalizedImageExt(sourceURL)
	switch ext {
	case ".gif":
		return imageKindGIF
	case ".jpg", ".jpeg", ".png":
		return imageKindStatic
	}

	contentType := strings.ToLower(http.DetectContentType(data))
	switch {
	case strings.Contains(contentType, "image/gif"):
		return imageKindGIF
	case strings.Contains(contentType, "image/jpeg"), strings.Contains(contentType, "image/jpg"), strings.Contains(contentType, "image/png"):
		return imageKindStatic
	}

	if _, err := gif.DecodeAll(bytes.NewReader(data)); err == nil {
		return imageKindGIF
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return imageKindStatic
	}
	return imageKindUnknown
}

func normalizedImageExt(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Path != "" {
		return strings.ToLower(path.Ext(parsed.Path))
	}
	return strings.ToLower(filepath.Ext(sourceURL))
}

func symmetricFileBase(messageID string, imageID int32) string {
	return sanitizeSymmetricToken(messageID) + "_" + strconv.FormatInt(int64(imageID), 10)
}

func sanitizeSymmetricToken(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "msg"
	}
	return b.String()
}

func makeSymmetricStatic(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			srcX := mirroredSourceX(bounds, x)
			dst.Set(x, y, src.At(srcX, y))
		}
	}
	return dst
}

func makeSymmetricGIF(src *gif.GIF) *gif.GIF {
	result := &gif.GIF{
		Image:           make([]*image.Paletted, 0, len(src.Image)),
		Delay:           append([]int(nil), src.Delay...),
		LoopCount:       src.LoopCount,
		Disposal:        append([]byte(nil), src.Disposal...),
		Config:          src.Config,
		BackgroundIndex: src.BackgroundIndex,
	}

	for _, frame := range src.Image {
		bounds := frame.Bounds()
		dst := image.NewPaletted(bounds, color.Palette(frame.Palette))
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				srcX := mirroredSourceX(bounds, x)
				dst.SetColorIndex(x, y, frame.ColorIndexAt(srcX, y))
			}
		}
		result.Image = append(result.Image, dst)
	}
	return result
}

func mirroredSourceX(bounds image.Rectangle, x int) int {
	width := bounds.Dx()
	leftWidth := (width + 1) / 2
	relativeX := x - bounds.Min.X
	if relativeX < leftWidth {
		return x
	}
	return bounds.Min.X + (width - 1 - relativeX)
}

func compactStringsInOrder(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}
