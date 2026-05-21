package imagestore

import (
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func TestSavePNGAddsExtensionAndOverwrites(t *testing.T) {
	client := NewClient(t.TempDir(), "http://example.com")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	if err := client.SavePNG(img, "sample"); err != nil {
		t.Fatalf("SavePNG returned error: %v", err)
	}
	if err := client.SavePNG(img, "sample"); err != nil {
		t.Fatalf("SavePNG overwrite returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(client.imagesDir, "sample.png")); err != nil {
		t.Fatalf("expected saved png to exist: %v", err)
	}
}

func TestSaveGIFKeepsExistingExtension(t *testing.T) {
	client := NewClient(t.TempDir(), "http://example.com")
	img := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White}),
		},
		Delay: []int{0},
	}

	if err := client.SaveGIF(img, "anim.GIF"); err != nil {
		t.Fatalf("SaveGIF returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(client.imagesDir, "anim.GIF")); err != nil {
		t.Fatalf("expected saved gif to exist: %v", err)
	}
}

func TestSaveRejectsInvalidNames(t *testing.T) {
	client := NewClient(t.TempDir(), "http://example.com")
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	invalidNames := []string{"bad-name", "bad name", "中文", "../bad", "bad.png.exe"}
	for _, name := range invalidNames {
		if err := client.SavePNG(img, name); err == nil {
			t.Fatalf("expected invalid name %q to fail", name)
		}
	}
}

func TestReadImageReturnsURL(t *testing.T) {
	client := NewClient(t.TempDir(), "http://example.com/")
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	if err := client.SavePNG(img, "avatar"); err != nil {
		t.Fatalf("SavePNG returned error: %v", err)
	}

	got, err := client.ReadImage("avatar.png")
	if err != nil {
		t.Fatalf("ReadImage returned error: %v", err)
	}
	if got != "http://example.com/images/avatar.png" {
		t.Fatalf("unexpected url: %s", got)
	}
}

func TestReadImageReturnsErrorWhenMissing(t *testing.T) {
	client := NewClient(t.TempDir(), "http://example.com")

	if _, err := client.ReadImage("missing.png"); err == nil {
		t.Fatal("expected missing image to return error")
	}
}

func TestReadImageReturnsErrorWhenMyURLMissing(t *testing.T) {
	client := NewClient(t.TempDir(), "")
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	if err := client.SavePNG(img, "avatar"); err != nil {
		t.Fatalf("SavePNG returned error: %v", err)
	}

	if _, err := client.ReadImage("avatar.png"); err == nil {
		t.Fatal("expected missing MY_URL to return error")
	}
}
