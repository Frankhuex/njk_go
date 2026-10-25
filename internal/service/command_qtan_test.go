package service

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math"
	"testing"

	"njk_go/internal/config"
)

func TestMatchCommandSupportsQTan(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:   "1558109748",
		BotNickname: "你居垦",
	}, nil, nil, nil, nil, nil, nil)

	valid := []struct {
		input     string
		wantCount string
		wantScale float64
		wantBPM   float64
	}{
		{input: ".Q弹 5", wantCount: "5", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".q弹5", wantCount: "5", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".Q弹5", wantCount: "5", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: "  .q弹  12  ", wantCount: "12", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".qtan 5", wantCount: "5", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".QTAN5", wantCount: "5", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".QTan 3", wantCount: "3", wantScale: qtanDefaultScale, wantBPM: qtanDefaultBPM},
		{input: ".qtan 10 scale2 bpm130", wantCount: "10", wantScale: 2, wantBPM: 130},
		{input: ".qtan10 scale 2 bpm 130", wantCount: "10", wantScale: 2, wantBPM: 130},
		{input: ".qtan 10 bpm130 scale2", wantCount: "10", wantScale: 2, wantBPM: 130},
		{input: ".qtan 10 bpm 130 scale 2", wantCount: "10", wantScale: 2, wantBPM: 130},
		{input: ".qtan 10 scale2.5bpm120", wantCount: "10", wantScale: 2.5, wantBPM: 120},
		{input: ".qtan 10 bpm130.5", wantCount: "10", wantScale: qtanDefaultScale, wantBPM: 130.5},
		{input: ".qtan 10 scale1.5 bpm128.5", wantCount: "10", wantScale: 1.5, wantBPM: 128.5},
		{input: ".qtan 10 bpm 99.9 scale 1.25", wantCount: "10", wantScale: 1.25, wantBPM: 99.9},
		{input: ".qtan 10 scale2", wantCount: "10", wantScale: 2, wantBPM: qtanDefaultBPM},
		{input: ".qtan 10 bpm90", wantCount: "10", wantScale: qtanDefaultScale, wantBPM: 90},
		{input: ".qtan 10 SCALE3 BPM150", wantCount: "10", wantScale: 3, wantBPM: 150},
	}
	for _, testCase := range valid {
		match := service.MatchCommand(testCase.input)
		if match == nil || match.Command.Key != commandQTan {
			t.Fatalf("expected %q to match Q弹 command, got=%v", testCase.input, match)
		}
		if len(match.Groups) < 2 || match.Groups[1] != testCase.wantCount {
			t.Fatalf("unexpected groups for %q: %#v", testCase.input, match.Groups)
		}
		scale, bpm, err := qtanParseOptions(match.Groups)
		if err != nil {
			t.Fatalf("qtanParseOptions(%q) failed: %v", testCase.input, err)
		}
		if scale != testCase.wantScale || bpm != testCase.wantBPM {
			t.Fatalf("%q parsed scale=%v bpm=%v want=%v/%v", testCase.input, scale, bpm, testCase.wantScale, testCase.wantBPM)
		}
	}

	invalid := []string{
		".Q弹", ".Q弹 abc", ".Q弹5.5", ".qtan", ".qtan abc",
		".qtan 10 scale2 scale3", ".qtan 10 bpm130 bpm140",
		".qtan 10 scale2 bpm130 scale3", ".qtan 10 foo2",
	}
	for _, input := range invalid {
		if match := service.MatchCommand(input); match != nil && match.Command.Key == commandQTan {
			t.Fatalf("expected %q not to match Q弹 command", input)
		}
	}
}

func TestQTanParseOptionsRanges(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:   "1558109748",
		BotNickname: "你居垦",
	}, nil, nil, nil, nil, nil, nil)

	for _, input := range []string{".qtan 5 scale1", ".qtan 5 scale11", ".qtan 5 bpm9", ".qtan 5 bpm1001"} {
		match := service.MatchCommand(input)
		if match == nil || match.Command.Key != commandQTan {
			t.Fatalf("expected %q to match Q弹 command", input)
		}
		if _, _, err := qtanParseOptions(match.Groups); err == nil {
			t.Fatalf("expected %q to fail range validation", input)
		}
	}
}

func TestQTanMotion(t *testing.T) {
	cases := []struct {
		bpm        float64
		wantBeat   int
		wantFrames int
	}{
		{bpm: 130, wantBeat: 460, wantFrames: 23},
		{bpm: 120, wantBeat: 500, wantFrames: 25},
		{bpm: 100, wantBeat: 600, wantFrames: 30},
		{bpm: 60, wantBeat: 1000, wantFrames: 50},
		{bpm: 1000, wantBeat: 60, wantFrames: 3},
	}
	for _, testCase := range cases {
		motion := newQTanMotion(qtanDefaultScale, testCase.bpm)
		if motion.beatMs != testCase.wantBeat || motion.frameCount != testCase.wantFrames {
			t.Fatalf("bpm=%v got beat=%d frames=%d want=%d/%d", testCase.bpm, motion.beatMs, motion.frameCount, testCase.wantBeat, testCase.wantFrames)
		}
	}
}

func TestQTanHeightFactor(t *testing.T) {
	motion := newQTanMotion(2, qtanDefaultBPM)
	if diff := math.Abs(motion.heightFactor(0) - 1); diff > 1e-9 {
		t.Fatalf("expected h(0)=1, got=%v", motion.heightFactor(0))
	}
	if diff := math.Abs(motion.heightFactor(1) - 1); diff > 1e-9 {
		t.Fatalf("expected h(1)=1, got=%v", motion.heightFactor(1))
	}

	maxValue := 0.0
	minValue := 10.0
	for i := 0; i <= 100000; i++ {
		value := motion.heightFactor(float64(i) / 100000)
		if value > maxValue {
			maxValue = value
		}
		if value < minValue {
			minValue = value
		}
	}
	if diff := math.Abs(maxValue - 2); diff > 1e-3 {
		t.Fatalf("expected max height factor 2, got=%v", maxValue)
	}
	if diff := math.Abs(minValue - 0.5); diff > 1e-3 {
		t.Fatalf("expected min height factor 0.5, got=%v", minValue)
	}

	if motion.heightFactor(0.2184) < 1.99 {
		t.Fatalf("expected first extremum to be the stretch peak, got=%v", motion.heightFactor(0.2184))
	}
	if motion.heightFactor(0.7184) > 0.51 {
		t.Fatalf("expected second extremum to be the compression trough, got=%v", motion.heightFactor(0.7184))
	}
	// QSpring 的三角相位在 cycle 中点已压到 2/3 原高。
	if diff := math.Abs(motion.heightFactor(0.5) - 2.0/3.0); diff > 1e-2 {
		t.Fatalf("expected h(0.5)≈0.667, got=%v", motion.heightFactor(0.5))
	}
}

func TestQTanHeightFactorWithScale(t *testing.T) {
	motion := newQTanMotion(3, qtanDefaultBPM)
	if diff := math.Abs(motion.heightFactor(0) - 1); diff > 1e-9 {
		t.Fatalf("expected h(0)=1 for scale 3, got=%v", motion.heightFactor(0))
	}
	maxValue := 0.0
	minValue := 10.0
	for i := 0; i <= 100000; i++ {
		value := motion.heightFactor(float64(i) / 100000)
		if value > maxValue {
			maxValue = value
		}
		if value < minValue {
			minValue = value
		}
	}
	if diff := math.Abs(maxValue - 3); diff > 1e-3 {
		t.Fatalf("expected max height factor 3, got=%v", maxValue)
	}
	if diff := math.Abs(minValue - 1.0/3.0); diff > 1e-3 {
		t.Fatalf("expected min height factor 1/3, got=%v", minValue)
	}
}

func TestQTanCubicInOut(t *testing.T) {
	cases := []struct {
		t    float64
		want float64
	}{
		{t: 0, want: 0},
		{t: 0.25, want: 0.0625},
		{t: 0.5, want: 0.5},
		{t: 0.75, want: 0.9375},
		{t: 1, want: 1},
	}
	for _, testCase := range cases {
		if diff := math.Abs(qtanCubicInOut(testCase.t) - testCase.want); diff > 1e-9 {
			t.Fatalf("qtanCubicInOut(%v)=%v want=%v", testCase.t, qtanCubicInOut(testCase.t), testCase.want)
		}
	}
}

func TestQTanBaseSizeFitsPixelBudget(t *testing.T) {
	cases := []struct {
		width      int
		height     int
		wantWidth  int
		wantHeight int
	}{
		{width: 1000, height: 1000, wantWidth: 95, wantHeight: 95},
		{width: 10, height: 10, wantWidth: 10, wantHeight: 10},
		{width: 1, height: 1, wantWidth: 1, wantHeight: 1},
		{width: 4000, height: 1, wantWidth: 4000, wantHeight: 1},
		{width: 1, height: 4000, wantWidth: 1, wantHeight: 909},
	}
	for _, testCase := range cases {
		gotWidth, gotHeight := qtanBaseSize(testCase.width, testCase.height, 2)
		if gotWidth != testCase.wantWidth || gotHeight != testCase.wantHeight {
			t.Fatalf("qtanBaseSize(%d,%d)=%d,%d want=%d,%d", testCase.width, testCase.height, gotWidth, gotHeight, testCase.wantWidth, testCase.wantHeight)
		}
		canvasWidth, canvasHeight := qtanCanvasSize(gotWidth, gotHeight, 2)
		if canvasWidth*canvasHeight > qtanMaxPixels {
			t.Fatalf("pixel budget exceeded for %dx%d: %d", testCase.width, testCase.height, canvasWidth*canvasHeight)
		}
	}
}

func TestQTanPaletteKeepsColorsForDominantColor(t *testing.T) {
	// 主导色恰好按颜色排序落在末尾，且权重超过一半。
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if (x+y)%10 < 7 {
				src.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				continue
			}
			src.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 128, A: 255})
		}
	}

	palette := qtanPalette(src)
	if len(palette) < 200 {
		t.Fatalf("expected full palette, got %d colors", len(palette))
	}
}

func TestBuildQTanGIFTransparentAndBottomAnchored(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				src.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}

	motion := newQTanMotion(2, qtanDefaultBPM)
	animated, err := buildQTanGIF(src, motion)
	if err != nil {
		t.Fatalf("buildQTanGIF failed: %v", err)
	}
	if len(animated.Image) != motion.frameCount {
		t.Fatalf("unexpected frame count: %d", len(animated.Image))
	}
	if animated.Config.Width != 28 || animated.Config.Height != 4 {
		t.Fatalf("unexpected config: %dx%d", animated.Config.Width, animated.Config.Height)
	}
	if animated.Config.Width*animated.Config.Height > qtanMaxPixels {
		t.Fatalf("pixel budget exceeded: %dx%d", animated.Config.Width, animated.Config.Height)
	}
	if animated.LoopCount != 0 {
		t.Fatalf("expected infinite loop, got=%d", animated.LoopCount)
	}
	if animated.BackgroundIndex != 0 {
		t.Fatalf("expected transparent background index 0, got=%d", animated.BackgroundIndex)
	}
	for i := range animated.Image {
		if animated.Delay[i] != qtanFrameDelay {
			t.Fatalf("unexpected delay at frame %d: %d", i, animated.Delay[i])
		}
		if animated.Disposal[i] != gif.DisposalBackground {
			t.Fatalf("unexpected disposal at frame %d: %d", i, animated.Disposal[i])
		}
		if len(animated.Image[i].Palette) > 256 {
			t.Fatalf("palette too large at frame %d: %d", i, len(animated.Image[i].Palette))
		}
	}

	if _, _, _, alpha := animated.Image[0].Palette[0].RGBA(); alpha != 0 {
		t.Fatalf("expected index 0 to be transparent, got alpha=%d", alpha)
	}
	if index := animated.Image[0].ColorIndexAt(0, 0); index != 0 {
		t.Fatalf("expected top-left corner transparent, got index=%d", index)
	}
	if index := animated.Image[0].ColorIndexAt(14, 3); index == 0 {
		t.Fatalf("expected bottom-anchored content at (14,3), got transparent index")
	}
	// 左右各 10px 透明留白。
	if index := animated.Image[0].ColorIndexAt(0, 3); index != 0 {
		t.Fatalf("expected left padding transparent, got index=%d", index)
	}
	if index := animated.Image[0].ColorIndexAt(animated.Config.Width-1, 3); index != 0 {
		t.Fatalf("expected right padding transparent, got index=%d", index)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animated); err != nil {
		t.Fatalf("gif.EncodeAll failed: %v", err)
	}
	decoded, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("gif.DecodeAll failed: %v", err)
	}
	if len(decoded.Image) != len(animated.Image) {
		t.Fatalf("decoded frame count=%d want=%d", len(decoded.Image), len(animated.Image))
	}
	if decoded.Config.Width != animated.Config.Width || decoded.Config.Height != animated.Config.Height {
		t.Fatalf("decoded config=%dx%d", decoded.Config.Width, decoded.Config.Height)
	}
}

func TestQTanAnimatedDuration(t *testing.T) {
	cases := []struct {
		source int
		beat   int
		want   int
	}{
		{source: 0, beat: 460, want: 460},
		{source: 230, beat: 460, want: 460},
		{source: 460, beat: 460, want: 460},
		{source: 920, beat: 460, want: 920},
		{source: 1380, beat: 460, want: 1380},
		{source: 6440, beat: 460, want: 6440},
		{source: 7820, beat: 460, want: 7820},
		{source: 500, beat: 460, want: 8280},
		{source: 1000, beat: 460, want: 8280},
		{source: 8001, beat: 460, want: 8280},
		{source: 8280, beat: 460, want: 8280},
		{source: 10000, beat: 460, want: 8280},
		{source: 500, beat: 500, want: 500},
		{source: 1000, beat: 500, want: 1000},
		{source: 700, beat: 500, want: 3500},
		{source: 900, beat: 500, want: 4500},
		{source: 1700, beat: 500, want: 8000},
	}
	for _, testCase := range cases {
		if got := qtanAnimatedDurationMs(testCase.source, testCase.beat); got != testCase.want {
			t.Fatalf("qtanAnimatedDurationMs(%d,%d)=%d want=%d", testCase.source, testCase.beat, got, testCase.want)
		}
	}
}

func TestBuildQTanAnimatedGIF(t *testing.T) {
	motion := newQTanMotion(2, qtanDefaultBPM)
	animated, err := buildQTanAnimatedGIF(qtanTestSourceGIF(t), motion)
	if err != nil {
		t.Fatalf("buildQTanAnimatedGIF failed: %v", err)
	}

	// 源时长 230ms，与 460ms 的最小公倍数为 460ms，对应 23 帧。
	if len(animated.Image) != motion.frameCount {
		t.Fatalf("unexpected frame count: %d", len(animated.Image))
	}
	if animated.Config.Width != 28 || animated.Config.Height != 4 {
		t.Fatalf("unexpected config: %dx%d", animated.Config.Width, animated.Config.Height)
	}
	if animated.Config.Width*animated.Config.Height > qtanMaxPixels {
		t.Fatalf("pixel budget exceeded: %dx%d", animated.Config.Width, animated.Config.Height)
	}
	if animated.BackgroundIndex != 0 {
		t.Fatalf("expected transparent background index 0, got=%d", animated.BackgroundIndex)
	}
	for i := range animated.Image {
		if animated.Delay[i] != qtanFrameDelay {
			t.Fatalf("unexpected delay at frame %d: %d", i, animated.Delay[i])
		}
		if animated.Disposal[i] != gif.DisposalBackground {
			t.Fatalf("unexpected disposal at frame %d: %d", i, animated.Disposal[i])
		}
	}
	if _, _, _, alpha := animated.Image[0].Palette[0].RGBA(); alpha != 0 {
		t.Fatalf("expected index 0 to be transparent, got alpha=%d", alpha)
	}
	if index := animated.Image[0].ColorIndexAt(0, 0); index != 0 {
		t.Fatalf("expected top-left corner transparent at frame 0, got index=%d", index)
	}

	// 第 0 帧对应源第 1 帧（红），第 6 帧（120ms）对应源第 2 帧（蓝）。
	if r, _, b := qtanTestPaletteColor(animated.Image[0], 14, 3); r < 0xc000 || b > 0x4000 {
		t.Fatalf("expected red source frame at (14,3), got r=%d b=%d", r, b)
	}
	if r, _, b := qtanTestPaletteColor(animated.Image[6], 14, 3); b < 0xc000 || r > 0x4000 {
		t.Fatalf("expected blue source frame at (14,3), got r=%d b=%d", r, b)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animated); err != nil {
		t.Fatalf("gif.EncodeAll failed: %v", err)
	}
	decoded, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("gif.DecodeAll failed: %v", err)
	}
	if len(decoded.Image) != len(animated.Image) {
		t.Fatalf("decoded frame count=%d want=%d", len(decoded.Image), len(animated.Image))
	}
}

func qtanTestPaletteColor(frame *image.Paletted, x int, y int) (uint32, uint32, uint32) {
	r, g, b, _ := frame.Palette[frame.ColorIndexAt(x, y)].RGBA()
	return r, g, b
}

func qtanTestSourceGIF(t *testing.T) []byte {
	t.Helper()

	palette := color.Palette{
		color.RGBA{},
		color.RGBA{R: 255, A: 255},
		color.RGBA{B: 255, A: 255},
		color.RGBA{},
	}
	first := image.NewPaletted(image.Rect(0, 0, 4, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 4, 2), palette)
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			first.SetColorIndex(x, y, 1)
			second.SetColorIndex(x, y, 2)
		}
	}

	source := &gif.GIF{
		Image:     []*image.Paletted{first, second},
		Delay:     []int{11, 12},
		Disposal:  []byte{gif.DisposalNone, gif.DisposalNone},
		LoopCount: 0,
		Config:    image.Config{ColorModel: palette, Width: 4, Height: 2},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, source); err != nil {
		t.Fatalf("gif.EncodeAll source failed: %v", err)
	}
	return buf.Bytes()
}
