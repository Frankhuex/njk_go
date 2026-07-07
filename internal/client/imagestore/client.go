package imagestore

import (
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type ImageStoreClient struct {
	imagesDir string
	myURL     string
}

func NewClient(baseDir string, myURL string) *ImageStoreClient {
	return &ImageStoreClient{
		imagesDir: ImagesDir(baseDir),
		myURL:     strings.TrimRight(myURL, "/"),
	}
}

func ImagesDir(baseDir string) string {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "."
	}
	return filepath.Join(baseDir, "images")
}

func (c *ImageStoreClient) SavePNG(img image.Image, fileName string) error {
	if img == nil {
		return fmt.Errorf("png image is nil")
	}

	resolvedName, err := normalizeSaveName(fileName, "png")
	if err != nil {
		return err
	}
	if err := ensureImagesDir(c.imagesDir); err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(c.imagesDir, resolvedName))
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func (c *ImageStoreClient) SaveGIF(img *gif.GIF, fileName string) error {
	if img == nil {
		return fmt.Errorf("gif image is nil")
	}

	resolvedName, err := normalizeSaveName(fileName, "gif")
	if err != nil {
		return err
	}
	if err := ensureImagesDir(c.imagesDir); err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(c.imagesDir, resolvedName))
	if err != nil {
		return err
	}
	defer file.Close()

	return gif.EncodeAll(file, img)
}

func (c *ImageStoreClient) ReadImage(fileName string) (string, error) {
	resolvedName, err := normalizeReadName(fileName)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(c.myURL) == "" {
		return "", fmt.Errorf("missing MY_URL")
	}

	path := filepath.Join(c.imagesDir, resolvedName)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("image not found: %s", resolvedName)
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("image path is a directory: %s", resolvedName)
	}

	return c.myURL + "/images/" + url.PathEscape(resolvedName), nil
}

func ensureImagesDir(imagesDir string) error {
	return os.MkdirAll(imagesDir, 0o755)
}

func normalizeSaveName(fileName string, expectedExt string) (string, error) {
	if err := validatePathSafety(fileName); err != nil {
		return "", err
	}

	ext := filepath.Ext(fileName)
	if ext != "" && !strings.EqualFold(ext, "."+expectedExt) {
		return "", fmt.Errorf("invalid file extension: %s", ext)
	}

	baseName := fileName
	if ext != "" {
		baseName = strings.TrimSuffix(fileName, ext)
	}
	if !fileNamePattern.MatchString(baseName) {
		return "", fmt.Errorf("invalid file name: %s", fileName)
	}

	if ext == "" {
		return baseName + "." + expectedExt, nil
	}
	return baseName + ext, nil
}

func normalizeReadName(fileName string) (string, error) {
	if err := validatePathSafety(fileName); err != nil {
		return "", err
	}

	ext := filepath.Ext(fileName)
	if ext == "" {
		return "", fmt.Errorf("missing file extension")
	}
	if !strings.EqualFold(ext, ".png") && !strings.EqualFold(ext, ".gif") {
		return "", fmt.Errorf("unsupported file extension: %s", ext)
	}

	baseName := strings.TrimSuffix(fileName, ext)
	if !fileNamePattern.MatchString(baseName) {
		return "", fmt.Errorf("invalid file name: %s", fileName)
	}

	return baseName + ext, nil
}

func validatePathSafety(fileName string) error {
	if fileName == "" {
		return fmt.Errorf("file name is empty")
	}
	if strings.Contains(fileName, "/") || strings.Contains(fileName, `\`) {
		return fmt.Errorf("file name must not contain path separators")
	}
	return nil
}
