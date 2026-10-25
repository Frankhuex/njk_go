package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/gif"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"

	"njk_go/internal/dal/model"
)

const (
	// 输出固定 50fps，每帧 20ms。
	qtanFrameDelay          = 2 // GIF 延迟单位为 1/100 秒
	qtanFrameIntervalMs     = 20
	qtanMaxAnimatedDuration = 8000
	qtanMaxSourcePixels     = 64 << 20
	qtanPaletteSampleFrames = 12
	qtanMaxPixels           = 40000
	qtanMaxConcurrency      = 4
	qtanTransparentIndex    = uint8(0)
	qtanAlphaThreshold      = uint8(128)
	qtanHorizontalPadding   = 10

	qtanDefaultScale = 1.5
	qtanDefaultBPM   = 130.0
	qtanMinScale     = 1.0
	qtanMaxScale     = 10.0
	qtanMinBPM       = 10.0
	qtanMaxBPM       = 1000.0
)

// qtanMotion 是一次请求使用的伸缩参数。
// beatMs 会被量化到 50fps 的 20ms 帧网格上，帧数由 bpm 推导。
type qtanMotion struct {
	scale       float64
	beatMs      int
	frameCount  int
	phaseOffset float64
}

func newQTanMotion(scale float64, bpm float64) qtanMotion {
	frames := int(math.Round(60000 / bpm / float64(qtanFrameIntervalMs)))
	if frames < 1 {
		frames = 1
	}
	return qtanMotion{
		scale:       scale,
		beatMs:      frames * qtanFrameIntervalMs,
		frameCount:  frames,
		phaseOffset: qtanPhaseOffsetFor(scale),
	}
}

// 与 QSpring 一致的 cubic inout：三角相位 + 缓动，高度取缩放系数的倒数。
// 相位偏移使 phase=0 时高度仍为原高。
func qtanPhaseOffsetFor(k float64) float64 {
	if k <= 1 {
		return 0
	}
	target := 1 / (k + 1)
	var t float64
	if target <= 0.5 {
		t = math.Cbrt(target / 4)
	} else {
		t = 1 - math.Cbrt((1-target)/4)
	}
	return 1 - t/2
}

func qtanCubicInOut(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := 1 - t
	return 1 - 4*u*u*u
}

func qtanParseOptions(groups []string) (float64, float64, error) {
	scaleText, bpmText := "", ""
	if len(groups) >= 8 {
		switch {
		case groups[2] != "":
			// scale ... bpm ...
			scaleText, bpmText = groups[2], groups[3]
		case groups[4] != "":
			// bpm ... scale ...
			bpmText, scaleText = groups[4], groups[5]
		case groups[6] != "":
			scaleText = groups[6]
		case groups[7] != "":
			bpmText = groups[7]
		}
	}

	scale := qtanDefaultScale
	if scaleText != "" {
		value, err := strconv.ParseFloat(scaleText, 64)
		if err != nil || value <= qtanMinScale || value > qtanMaxScale {
			return 0, 0, fmt.Errorf("scale 需大于 %g 且不超过 %g", qtanMinScale, qtanMaxScale)
		}
		scale = value
	}

	bpm := qtanDefaultBPM
	if bpmText != "" {
		value, err := strconv.ParseFloat(bpmText, 64)
		if err != nil || value < qtanMinBPM || value > qtanMaxBPM {
			return 0, 0, fmt.Errorf("bpm 需在 %g 到 %g 之间", qtanMinBPM, qtanMaxBPM)
		}
		bpm = value
	}

	return scale, bpm, nil
}

func (s *Service) handleQTanCommand(ctx context.Context, groupID string, match CommandMatch) (*OutboundAction, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	count, err := strconv.Atoi(match.Groups[1])
	if err != nil || count < 1 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	scale, bpm, err := qtanParseOptions(match.Groups)
	if err != nil {
		return simpleOutbound(groupID, err.Error()), nil
	}
	motion := newQTanMotion(scale, bpm)

	images, err := s.store.RecentMessageImages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return simpleOutbound(groupID, "最近消息里没有图片"), nil
	}

	outputURLs := make([]string, len(images))
	sem := make(chan struct{}, qtanMaxConcurrency)
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

			data, err := s.DownloadImageBytes(ctx, *item.URL)
			if err != nil {
				return
			}

			outputURL, err := s.makeQTanImage(item, data, *item.URL, motion)
			if err != nil {
				return
			}

			mu.Lock()
			outputURLs[idx] = outputURL
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

func (s *Service) makeQTanImage(item model.Image, data []byte, sourceURL string, motion qtanMotion) (string, error) {
	var animated *gif.GIF
	var err error

	switch detectImageKind(sourceURL, data) {
	case imageKindGIF:
		animated, err = buildQTanAnimatedGIF(data, motion)
	case imageKindStatic:
		src, _, decodeErr := image.Decode(bytes.NewReader(data))
		if decodeErr != nil {
			return "", decodeErr
		}
		animated, err = buildQTanGIF(src, motion)
	default:
		return "", fmt.Errorf("unsupported image type")
	}
	if err != nil {
		return "", err
	}

	fileName := qtanFileBase(item.MessageID, item.ID) + ".gif"
	if err := s.imageStore.SaveGIF(animated, fileName); err != nil {
		return "", err
	}
	return s.imageStore.ReadImage(fileName)
}

func qtanFileBase(messageID string, imageID int32) string {
	return "qtan_" + sanitizeSymmetricToken(messageID) + "_" + strconv.FormatInt(int64(imageID), 10)
}

// heightFactor 返回 QSpring cubic inout 下的高度倍数。
// phase 为 0 时高度为原高，随后升至振幅上限、降到底部再回到原高。
func (m qtanMotion) heightFactor(phase float64) float64 {
	if m.scale <= 1 {
		return 1
	}
	p := math.Mod(phase+m.phaseOffset, 1)
	if p < 0 {
		p++
	}
	t := 2 * p
	if t > 1 {
		t = 2 - t
	}
	scale := 1/m.scale + (m.scale-1/m.scale)*qtanCubicInOut(t)
	return 1 / scale
}

// qtanBaseSize 返回原图缩放后的基准宽高。画布为 k*W x k*H，保证最终像素数不超过 qtanMaxPixels。
func qtanBaseSize(width int, height int, scaleValue float64) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}

	scale := math.Sqrt(float64(qtanMaxPixels) / (scaleValue * scaleValue * float64(width) * float64(height)))
	if scale > 1 {
		scale = 1
	}
	baseWidth := int(math.Round(float64(width) * scale))
	baseHeight := int(math.Round(float64(height) * scale))
	if baseWidth < 1 {
		baseWidth = 1
	}
	if baseHeight < 1 {
		baseHeight = 1
	}

	for {
		canvasWidth, canvasHeight := qtanCanvasSize(baseWidth, baseHeight, scaleValue)
		if canvasWidth*canvasHeight <= qtanMaxPixels {
			break
		}
		if baseWidth >= baseHeight && baseWidth > 1 {
			baseWidth--
			continue
		}
		if baseHeight > 1 {
			baseHeight--
			continue
		}
		break
	}
	return baseWidth, baseHeight
}

// qtanCanvasSize 返回画布尺寸：左右各留 qtanHorizontalPadding 像素空白。
func qtanCanvasSize(baseWidth int, baseHeight int, scaleValue float64) (int, int) {
	return int(math.Ceil(float64(baseWidth)*scaleValue)) + 2*qtanHorizontalPadding, int(math.Ceil(float64(baseHeight) * scaleValue))
}

func buildQTanGIF(src image.Image, motion qtanMotion) (*gif.GIF, error) {
	srcBounds := src.Bounds()
	baseWidth, baseHeight := qtanBaseSize(srcBounds.Dx(), srcBounds.Dy(), motion.scale)
	if baseWidth <= 0 || baseHeight <= 0 {
		return nil, fmt.Errorf("invalid image size")
	}

	base := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
	xdraw.CatmullRom.Scale(base, base.Bounds(), src, srcBounds, xdraw.Src, nil)

	palette := qtanPalette(base)
	canvasWidth, canvasHeight := qtanCanvasSize(baseWidth, baseHeight, motion.scale)
	canvas := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	frames := make([]*image.Paletted, 0, motion.frameCount)
	colorCache := make(map[uint32]uint8, 512)
	area := baseWidth * baseHeight

	for index := 0; index < motion.frameCount; index++ {
		stddraw.Draw(canvas, canvas.Bounds(), image.Transparent, image.Point{}, stddraw.Src)

		t := float64(index) / float64(motion.frameCount)
		height := int(math.Round(float64(baseHeight) * motion.heightFactor(t)))
		if height < 1 {
			height = 1
		}
		if height > canvasHeight {
			height = canvasHeight
		}
		width := int(math.Round(float64(area) / float64(height)))
		if width < 1 {
			width = 1
		}
		if width > canvasWidth-2*qtanHorizontalPadding {
			width = canvasWidth - 2*qtanHorizontalPadding
		}

		x := (canvasWidth - width) / 2
		y := canvasHeight - height
		xdraw.CatmullRom.Scale(canvas, image.Rect(x, y, x+width, y+height), base, base.Bounds(), xdraw.Src, nil)
		frames = append(frames, qtanPaletted(canvas, palette, colorCache))
	}

	delay := make([]int, len(frames))
	disposal := make([]byte, len(frames))
	for i := range frames {
		delay[i] = qtanFrameDelay
		disposal[i] = gif.DisposalBackground
	}

	return &gif.GIF{
		Image:           frames,
		Delay:           delay,
		Disposal:        disposal,
		LoopCount:       0,
		Config:          image.Config{ColorModel: palette, Width: canvasWidth, Height: canvasHeight},
		BackgroundIndex: qtanTransparentIndex,
	}, nil
}

// qtanAnimatedDurationMs 计算动图输出时长。
// 源 GIF 周期与一拍的最小公倍数不超过 8s 时取公倍数，两个循环都无缝；
// 超过 8s 时取 8s 之后第一个 Q弹 周期结束点，源动画在该点硬截断。
func qtanAnimatedDurationMs(sourceDurationMs int, beatMs int) int {
	if sourceDurationMs <= 0 {
		return beatMs
	}
	if sourceDurationMs <= qtanMaxAnimatedDuration {
		if common := qtanLCM(sourceDurationMs, beatMs); common <= qtanMaxAnimatedDuration {
			return common
		}
	}
	return ((qtanMaxAnimatedDuration + beatMs - 1) / beatMs) * beatMs
}

func qtanLCM(a int, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	return a / qtanGCD(a, b) * b
}

func qtanGCD(a int, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// buildQTanAnimatedGIF 把源 GIF 的每一帧按 Q弹 曲线变换后重新编码。
// 源动画保持原速播放，输出时间轴固定 50fps。
func buildQTanAnimatedGIF(data []byte, motion qtanMotion) (*gif.GIF, error) {
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(decoded.Image) == 0 {
		return nil, fmt.Errorf("empty gif")
	}

	width, height := qtanGIFSize(decoded)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid gif size")
	}
	if int64(width)*int64(height)*int64(len(decoded.Image)) > qtanMaxSourcePixels {
		return nil, fmt.Errorf("gif too large")
	}

	delays := make([]int, len(decoded.Image))
	totalDuration := 0
	for index := range decoded.Image {
		delay := 10
		if index < len(decoded.Delay) && decoded.Delay[index] > 0 {
			delay = decoded.Delay[index] * 10
		}
		delays[index] = delay
		totalDuration += delay
	}
	outputDuration := qtanAnimatedDurationMs(totalDuration, motion.beatMs)

	baseWidth, baseHeight := qtanBaseSize(width, height, motion.scale)
	if baseWidth <= 0 || baseHeight <= 0 {
		return nil, fmt.Errorf("invalid image size")
	}

	baseFrames, frameStarts := qtanCompositedBaseFrames(decoded, delays, width, height, baseWidth, baseHeight, outputDuration)
	if len(baseFrames) == 0 {
		return nil, fmt.Errorf("empty gif")
	}

	palette := qtanPaletteFromFrames(baseFrames)
	canvasWidth, canvasHeight := qtanCanvasSize(baseWidth, baseHeight, motion.scale)
	canvas := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	colorCache := make(map[uint32]uint8, 512)
	area := baseWidth * baseHeight
	// 输出时长是一拍的整数倍，而一拍已量化到 20ms 网格，因此帧间隔始终均匀。
	frameCount := outputDuration / qtanFrameIntervalMs
	frames := make([]*image.Paletted, 0, frameCount)
	delay := make([]int, frameCount)
	disposal := make([]byte, frameCount)

	for index := 0; index < frameCount; index++ {
		elapsed := index * qtanFrameIntervalMs
		sourceIndex := qtanSourceFrameIndex(frameStarts, elapsed%totalDuration)
		phase := float64(elapsed%motion.beatMs) / float64(motion.beatMs)

		stddraw.Draw(canvas, canvas.Bounds(), image.Transparent, image.Point{}, stddraw.Src)

		height := int(math.Round(float64(baseHeight) * motion.heightFactor(phase)))
		if height < 1 {
			height = 1
		}
		if height > canvasHeight {
			height = canvasHeight
		}
		width := int(math.Round(float64(area) / float64(height)))
		if width < 1 {
			width = 1
		}
		if width > canvasWidth-2*qtanHorizontalPadding {
			width = canvasWidth - 2*qtanHorizontalPadding
		}

		x := (canvasWidth - width) / 2
		y := canvasHeight - height
		base := baseFrames[sourceIndex]
		xdraw.CatmullRom.Scale(canvas, image.Rect(x, y, x+width, y+height), base, base.Bounds(), xdraw.Src, nil)
		frames = append(frames, qtanPaletted(canvas, palette, colorCache))
		delay[index] = qtanFrameDelay
		disposal[index] = gif.DisposalBackground
	}

	return &gif.GIF{
		Image:           frames,
		Delay:           delay,
		Disposal:        disposal,
		LoopCount:       0,
		Config:          image.Config{ColorModel: palette, Width: canvasWidth, Height: canvasHeight},
		BackgroundIndex: qtanTransparentIndex,
	}, nil
}

func qtanGIFSize(decoded *gif.GIF) (int, int) {
	if decoded.Config.Width > 0 && decoded.Config.Height > 0 {
		return decoded.Config.Width, decoded.Config.Height
	}
	if len(decoded.Image) == 0 {
		return 0, 0
	}
	bounds := decoded.Image[0].Bounds()
	return bounds.Max.X, bounds.Max.Y
}

// qtanCompositedBaseFrames 按 GIF 的 Disposal 规则把子帧合成为完整画布，
// 再统一缩放到 Q弹 的基准尺寸，避免在输出阶段反复处理源分辨率。
func qtanCompositedBaseFrames(decoded *gif.GIF, delays []int, width int, height int, baseWidth int, baseHeight int, limitMs int) ([]*image.RGBA, []int) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	frames := make([]*image.RGBA, 0, len(decoded.Image))
	starts := make([]int, 0, len(decoded.Image))
	elapsed := 0

	for index, frame := range decoded.Image {
		if elapsed >= limitMs {
			break
		}

		disposal := byte(gif.DisposalNone)
		if index < len(decoded.Disposal) {
			disposal = decoded.Disposal[index]
		}

		var previous []byte
		if disposal == gif.DisposalPrevious {
			previous = append([]byte(nil), canvas.Pix...)
		}

		stddraw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, stddraw.Over)
		scaled := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), canvas, canvas.Bounds(), xdraw.Src, nil)
		frames = append(frames, scaled)
		starts = append(starts, elapsed)

		switch disposal {
		case gif.DisposalBackground:
			stddraw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, stddraw.Src)
		case gif.DisposalPrevious:
			if previous != nil {
				copy(canvas.Pix, previous)
			}
		}

		elapsed += delays[index]
	}
	return frames, starts
}

func qtanSourceFrameIndex(starts []int, elapsed int) int {
	index := sort.Search(len(starts), func(i int) bool {
		return starts[i] > elapsed
	}) - 1
	if index < 0 {
		return 0
	}
	return index
}

func qtanPalette(base image.Image) color.Palette {
	counts := make(map[uint32]int)
	qtanAccumulateColors(counts, base)
	return qtanPaletteFromCounts(counts)
}

func qtanPaletteFromFrames(frames []*image.RGBA) color.Palette {
	counts := make(map[uint32]int)
	if len(frames) == 0 {
		return qtanPaletteFromCounts(counts)
	}

	step := 1
	if len(frames) > qtanPaletteSampleFrames {
		step = (len(frames) + qtanPaletteSampleFrames - 1) / qtanPaletteSampleFrames
	}
	for index := 0; index < len(frames); index += step {
		qtanAccumulateColors(counts, frames[index])
	}
	qtanAccumulateColors(counts, frames[len(frames)-1])
	return qtanPaletteFromCounts(counts)
}

func qtanAccumulateColors(counts map[uint32]int, src image.Image) {
	bounds := src.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := qtanRGBA8(src.At(x, y))
			if a < qtanAlphaThreshold {
				continue
			}
			counts[uint32(r)<<16|uint32(g)<<8|uint32(b)]++
		}
	}
}

func qtanPaletteFromCounts(counts map[uint32]int) color.Palette {
	entries := make([]qtanColorEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, qtanColorEntry{
			r:     key >> 16 & 0xff,
			g:     key >> 8 & 0xff,
			b:     key & 0xff,
			count: count,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return qtanColorKey(entries[i]) < qtanColorKey(entries[j])
	})
	entries = qtanQuantizeEntries(entries, 255)

	palette := make(color.Palette, 0, len(entries)+1)
	palette = append(palette, color.RGBA{})
	for _, entry := range entries {
		palette = append(palette, color.RGBA{
			R: uint8(entry.r),
			G: uint8(entry.g),
			B: uint8(entry.b),
			A: 255,
		})
	}
	if len(palette) < 2 {
		palette = append(palette, color.RGBA{A: 255})
	}
	return palette
}

func qtanPaletted(src *image.RGBA, palette color.Palette, cache map[uint32]uint8) *image.Paletted {
	bounds := src.Bounds()
	dst := image.NewPaletted(bounds, palette)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := qtanRGBA8(src.At(x, y))
			if a < qtanAlphaThreshold {
				dst.SetColorIndex(x, y, qtanTransparentIndex)
				continue
			}
			key := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
			index, ok := cache[key]
			if !ok {
				index = qtanNearestColor(palette, r, g, b)
				cache[key] = index
			}
			dst.SetColorIndex(x, y, index)
		}
	}
	return dst
}

func qtanNearestColor(palette color.Palette, r uint8, g uint8, b uint8) uint8 {
	bestIndex := 1
	bestDistance := math.MaxInt
	for i := 1; i < len(palette); i++ {
		cr, cg, cb, _ := qtanRGBA8(palette[i])
		dr := int(r) - int(cr)
		dg := int(g) - int(cg)
		db := int(b) - int(cb)
		distance := dr*dr + dg*dg + db*db
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = i
		}
	}
	return uint8(bestIndex)
}

func qtanRGBA8(c color.Color) (uint8, uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return 0, 0, 0, 0
	}
	r = r * 0xffff / a
	g = g * 0xffff / a
	b = b * 0xffff / a
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)
}

type qtanColorEntry struct {
	r     uint32
	g     uint32
	b     uint32
	count int
}

func qtanColorKey(entry qtanColorEntry) uint32 {
	return entry.r<<16 | entry.g<<8 | entry.b
}

func qtanQuantizeEntries(entries []qtanColorEntry, maxColors int) []qtanColorEntry {
	if len(entries) <= maxColors {
		return entries
	}

	boxes := [][]qtanColorEntry{entries}
	for len(boxes) < maxColors {
		boxIndex := -1
		bestRange := -1
		channel := 0
		for index, box := range boxes {
			if len(box) < 2 {
				continue
			}
			minR, maxR, minG, maxG, minB, maxB := qtanBoxBounds(box)
			rangeR := maxR - minR
			rangeG := maxG - minG
			rangeB := maxB - minB
			boxRange := qtanMax3(rangeR, rangeG, rangeB)
			if boxRange <= bestRange {
				continue
			}
			bestRange = boxRange
			boxIndex = index
			switch {
			case rangeR >= rangeG && rangeR >= rangeB:
				channel = 0
			case rangeG >= rangeB:
				channel = 1
			default:
				channel = 2
			}
		}
		if boxIndex < 0 || bestRange <= 0 {
			break
		}

		box := boxes[boxIndex]
		sort.SliceStable(box, func(i, j int) bool {
			valueI := qtanChannelValue(box[i], channel)
			valueJ := qtanChannelValue(box[j], channel)
			if valueI != valueJ {
				return valueI < valueJ
			}
			return qtanColorKey(box[i]) < qtanColorKey(box[j])
		})

		total := 0
		for _, entry := range box {
			total += entry.count
		}
		// 选择使左右权重最接近的切分点。不能只找“累计达到一半”的位置：
		// 当某个主导色恰好排在末尾且权重超过一半时，这种写法会得到 split=0，
		// 进而让整个量化提前退出，调色板退化成几个平均色。
		split := 0
		bestDiff := total + 1
		accumulated := 0
		for i := 0; i < len(box)-1; i++ {
			accumulated += box[i].count
			diff := accumulated*2 - total
			if diff < 0 {
				diff = -diff
			}
			if diff < bestDiff {
				bestDiff = diff
				split = i + 1
			}
		}
		if split <= 0 || split >= len(box) {
			break
		}
		boxes[boxIndex] = box[:split]
		boxes = append(boxes, box[split:])
	}

	result := make([]qtanColorEntry, 0, len(boxes))
	for _, box := range boxes {
		if len(box) == 0 {
			continue
		}
		var sumR, sumG, sumB, sumCount int
		for _, entry := range box {
			sumR += int(entry.r) * entry.count
			sumG += int(entry.g) * entry.count
			sumB += int(entry.b) * entry.count
			sumCount += entry.count
		}
		if sumCount == 0 {
			continue
		}
		result = append(result, qtanColorEntry{
			r:     uint32(sumR / sumCount),
			g:     uint32(sumG / sumCount),
			b:     uint32(sumB / sumCount),
			count: sumCount,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return qtanColorKey(result[i]) < qtanColorKey(result[j])
	})
	return result
}

func qtanBoxBounds(box []qtanColorEntry) (int, int, int, int, int, int) {
	minR, maxR := int(box[0].r), int(box[0].r)
	minG, maxG := int(box[0].g), int(box[0].g)
	minB, maxB := int(box[0].b), int(box[0].b)
	for _, entry := range box[1:] {
		r, g, b := int(entry.r), int(entry.g), int(entry.b)
		minR = minInt(minR, r)
		maxR = maxInt(maxR, r)
		minG = minInt(minG, g)
		maxG = maxInt(maxG, g)
		minB = minInt(minB, b)
		maxB = maxInt(maxB, b)
	}
	return minR, maxR, minG, maxG, minB, maxB
}

func qtanChannelValue(entry qtanColorEntry, channel int) uint32 {
	switch channel {
	case 1:
		return entry.g
	case 2:
		return entry.b
	default:
		return entry.r
	}
}

func qtanMax3(a int, b int, c int) int {
	return maxInt(maxInt(a, b), c)
}
