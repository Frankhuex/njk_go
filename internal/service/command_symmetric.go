package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"njk_go/internal/dal/model"
	"njk_go/internal/util/uimage"
)

type imageKind int

const (
	imageKindUnknown imageKind = iota
	imageKindStatic
	imageKindGIF
)

type symmetryMode int

const (
	symmetryLeft symmetryMode = iota
	symmetryRight
	symmetryUp
	symmetryDown
	symmetryLeftUp
	symmetryRightUp
	symmetryLeftDown
	symmetryRightDown
)

const symmetricImageMaxConcurrency = 4

func (s *Service) handleSymmetricCommand(ctx context.Context, groupID string, match CommandMatch) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	mode, err := symmetryModeFromCommand(match.Command.Key)
	if err != nil {
		return nil, err
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

			imageURL, err := s.makeSymmetricImage(ctx, mode, item, data, *item.URL)
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

func symmetryModeFromCommand(key commandKey) (symmetryMode, error) {
	switch key {
	case commandSymmetricLeft:
		return symmetryLeft, nil
	case commandSymmetricRight:
		return symmetryRight, nil
	case commandSymmetricUp:
		return symmetryUp, nil
	case commandSymmetricDown:
		return symmetryDown, nil
	case commandSymmetricLeftUp:
		return symmetryLeftUp, nil
	case commandSymmetricRightUp:
		return symmetryRightUp, nil
	case commandSymmetricLeftDown:
		return symmetryLeftDown, nil
	case commandSymmetricRightDown:
		return symmetryRightDown, nil
	default:
		return symmetryLeft, fmt.Errorf("unsupported symmetry command: %s", key)
	}
}

func (s *Service) makeSymmetricImage(ctx context.Context, mode symmetryMode, item model.Image, data []byte, sourceURL string) (string, error) {
	kind := detectImageKind(sourceURL, data)
	baseName := symmetricFileBase(item.MessageID, item.ID)

	switch kind {
	case imageKindGIF:
		animated, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		result := makeSymmetricGIF(animated, mode)
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
		result := makeSymmetricStatic(img, mode)
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
	ext := uimage.NormalizedExt(sourceURL)
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

func makeSymmetricStatic(src image.Image, mode symmetryMode) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			srcX, srcY := mirroredSourcePoint(bounds, x, y, mode)
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func makeSymmetricGIF(src *gif.GIF, mode symmetryMode) *gif.GIF {
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
				srcX, srcY := mirroredSourcePoint(bounds, x, y, mode)
				dst.SetColorIndex(x, y, frame.ColorIndexAt(srcX, srcY))
			}
		}
		result.Image = append(result.Image, dst)
	}
	return result
}

func mirroredSourcePoint(bounds image.Rectangle, x int, y int, mode symmetryMode) (int, int) {
	width := bounds.Dx()
	height := bounds.Dy()
	relativeX := x - bounds.Min.X
	relativeY := y - bounds.Min.Y

	leftX := relativeX
	rightX := width - 1 - relativeX
	upY := relativeY
	downY := height - 1 - relativeY

	switch mode {
	case symmetryLeft:
		return bounds.Min.X + minInt(leftX, rightX), y
	case symmetryRight:
		return bounds.Min.X + maxInt(leftX, rightX), y
	case symmetryUp:
		return x, bounds.Min.Y + minInt(upY, downY)
	case symmetryDown:
		return x, bounds.Min.Y + maxInt(upY, downY)
	case symmetryLeftUp:
		return bounds.Min.X + minInt(leftX, rightX), bounds.Min.Y + minInt(upY, downY)
	case symmetryRightUp:
		return bounds.Min.X + maxInt(leftX, rightX), bounds.Min.Y + minInt(upY, downY)
	case symmetryLeftDown:
		return bounds.Min.X + minInt(leftX, rightX), bounds.Min.Y + maxInt(upY, downY)
	case symmetryRightDown:
		return bounds.Min.X + maxInt(leftX, rightX), bounds.Min.Y + maxInt(upY, downY)
	default:
		return x, y
	}
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
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
