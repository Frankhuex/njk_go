package bot

import (
	"image"
	"image/color"
	"image/gif"
	"testing"

	"njk_go/internal/config"
)

func TestMatchCommandSupportsSymmetricWithoutSpace(t *testing.T) {
	service := NewService(config.Config{
		BotUserID:       "1558109748",
		BotNickname:     "你居垦",
		AllowedGroupIDs: map[string]struct{}{},
	}, nil, nil, nil, nil)

	match := service.matchCommand(".对称左5")
	if match == nil || match.Command.Key != commandSymmetricLeft {
		t.Fatalf("expected .对称左5 to match symmetric command, got=%v", match)
	}
	if len(match.Groups) < 2 || match.Groups[1] != "5" {
		t.Fatalf("unexpected groups: %#v", match.Groups)
	}
}

func TestMakeSymmetricStaticMirrorsRightHalf(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 1))
	colors := []color.RGBA{
		{R: 1, A: 255},
		{R: 2, A: 255},
		{R: 3, A: 255},
		{R: 4, A: 255},
		{R: 5, A: 255},
	}
	for x, c := range colors {
		src.Set(x, 0, c)
	}

	got := makeSymmetricStatic(src)
	want := []uint8{1, 2, 3, 2, 1}
	for x, expected := range want {
		r, _, _, _ := got.At(x, 0).RGBA()
		if uint8(r>>8) != expected {
			t.Fatalf("unexpected pixel at x=%d: got=%d want=%d", x, uint8(r>>8), expected)
		}
	}
}

func TestMakeSymmetricGIFMirrorsEachFrame(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 4, 1), color.Palette{
		color.RGBA{R: 1, A: 255},
		color.RGBA{R: 2, A: 255},
		color.RGBA{R: 3, A: 255},
		color.RGBA{R: 4, A: 255},
	})
	frame.SetColorIndex(0, 0, 0)
	frame.SetColorIndex(1, 0, 1)
	frame.SetColorIndex(2, 0, 2)
	frame.SetColorIndex(3, 0, 3)

	src := &gif.GIF{
		Image:     []*image.Paletted{frame},
		Delay:     []int{7},
		LoopCount: 2,
	}

	got := makeSymmetricGIF(src)
	if len(got.Image) != 1 {
		t.Fatalf("unexpected frame count: %d", len(got.Image))
	}
	if got.Delay[0] != 7 || got.LoopCount != 2 {
		t.Fatalf("gif metadata not preserved: delay=%v loop=%d", got.Delay, got.LoopCount)
	}

	want := []uint8{0, 1, 1, 0}
	for x, expected := range want {
		if index := got.Image[0].ColorIndexAt(x, 0); index != expected {
			t.Fatalf("unexpected color index at x=%d: got=%d want=%d", x, index, expected)
		}
	}
}

func TestDetectImageKindSupportsAliasesAndCase(t *testing.T) {
	gifData := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x00\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	if kind := detectImageKind("https://example.com/a.JPEG", []byte{0x89, 0x50, 0x4e, 0x47}); kind != imageKindStatic {
		t.Fatalf("expected jpeg alias to be treated as static, got=%v", kind)
	}
	if kind := detectImageKind("https://example.com/a.GIF", gifData); kind != imageKindGIF {
		t.Fatalf("expected gif alias to be treated as gif, got=%v", kind)
	}
}

func TestCompactStringsInOrder(t *testing.T) {
	got := compactStringsInOrder([]string{"a", "", "b", "", "c"})
	if len(got) != 3 {
		t.Fatalf("unexpected length: %d", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected order: %#v", got)
	}
}
