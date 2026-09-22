package modedit

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // registers the decoders image.Decode uses
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ThumbnailMaxSide is the longest side, in pixels, a saved thumbnail can have: a bigger picture is
	// scaled down to it (never a smaller one up), keeping its proportions.
	ThumbnailMaxSide = 512
	// ThumbnailFile is the name the thumbnail is saved under in the mod's folder.
	ThumbnailFile = "thumbnail.png"

	maxSourceBytes  = 20 << 20
	maxSourceSide   = 8192
	maxSourcePixels = 50_000_000
	// warnThumbnailBytes is the size above which the saved picture gets a warning: Steam limits the
	// preview image of a Workshop item.
	warnThumbnailBytes = 1 << 20
)

// Thumbnail is a picture ready to be saved as a mod's thumbnail.
type Thumbnail struct {
	// PNG is the picture as it will be written.
	PNG []byte
	// SourceWidth and SourceHeight are the chosen picture's size; Width and Height the saved one's.
	SourceWidth, SourceHeight int
	Width, Height             int
	// SourceBytes is the chosen file's size.
	SourceBytes int64
	Resized     bool
	Warnings    []string
}

// PrepareThumbnail reads a PNG, JPEG or GIF picture, scales it down to ThumbnailMaxSide if it is
// larger, and encodes it as a PNG. A file that is not a picture, or is unreasonably large, is refused
// before it is decoded.
func PrepareThumbnail(path string) (Thumbnail, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Thumbnail{}, fmt.Errorf("modedit: the picture cannot be read: %w", err)
	}
	if info.IsDir() {
		return Thumbnail{}, errors.New("modedit: that is a folder, not a picture")
	}
	if info.Size() > maxSourceBytes {
		return Thumbnail{}, fmt.Errorf("modedit: the picture is %d MB; pictures over %d MB are not accepted", info.Size()>>20, maxSourceBytes>>20)
	}
	data, err := readLimited(path)
	if err != nil {
		return Thumbnail{}, fmt.Errorf("modedit: the picture cannot be read: %w", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Thumbnail{}, fmt.Errorf("modedit: %s is not a PNG, JPEG or GIF picture", filepath.Base(path))
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSourceSide || cfg.Height > maxSourceSide || cfg.Width*cfg.Height > maxSourcePixels {
		return Thumbnail{}, fmt.Errorf("modedit: a %dx%d picture is too large to use (at most %d pixels a side)", cfg.Width, cfg.Height, maxSourceSide)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Thumbnail{}, fmt.Errorf("modedit: %s could not be read as a picture: %w", filepath.Base(path), err)
	}

	out := Thumbnail{SourceWidth: cfg.Width, SourceHeight: cfg.Height, Width: cfg.Width, Height: cfg.Height, SourceBytes: info.Size()}
	scaled := src
	if longest := max(cfg.Width, cfg.Height); longest > ThumbnailMaxSide {
		w, h := scaledSize(cfg.Width, cfg.Height, ThumbnailMaxSide)
		scaled = scaleDown(src, w, h)
		out.Width, out.Height, out.Resized = w, h, true
	} else if _, isPNG := src.(*image.NRGBA); !isPNG {
		// Encoding needs nothing but the pixels; draw onto a plain image so any source type encodes.
		b := src.Bounds()
		plain := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(plain, plain.Bounds(), src, b.Min, draw.Src)
		scaled = plain
	}

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, scaled); err != nil {
		return Thumbnail{}, fmt.Errorf("modedit: encoding the thumbnail: %w", err)
	}
	out.PNG = buf.Bytes()
	if len(out.PNG) > warnThumbnailBytes {
		out.Warnings = append(out.Warnings, fmt.Sprintf("The thumbnail is %.1f MB. Steam limits a Workshop preview image to 1 MB, so it may be refused when uploaded.", float64(len(out.PNG))/(1<<20)))
	}
	return out, nil
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxSourceBytes+1))
}

// scaledSize is the size of a w x h picture scaled so its longest side is side.
func scaledSize(w, h, side int) (int, int) {
	if w >= h {
		return side, max(1, (h*side+w/2)/w)
	}
	return max(1, (w*side+h/2)/h), side
}

// scaleDown shrinks src to w x h by averaging the source pixels each new pixel covers (a box
// filter, which is what keeps a large picture from turning jagged). Colours are averaged with their
// transparency, so a picture with see-through parts stays correct.
func scaleDown(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, max(y*sh/h+1, (y+1)*sh/h)
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max(x*sw/w+1, (x+1)*sw/w)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1 && sy < sh; sy++ {
				for sx := x0; sx < x1 && sx < sw; sx++ {
					pr, pg, pb, pa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA() // premultiplied, 16 bit
					r += uint64(pr)
					g += uint64(pg)
					bl += uint64(pb)
					a += uint64(pa)
					n++
				}
			}
			if n == 0 || a == 0 {
				continue
			}
			// Un-premultiply the averaged colour into 8-bit NRGBA.
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8((r * 0xffff / a) >> 8)
			dst.Pix[i+1] = uint8((g * 0xffff / a) >> 8)
			dst.Pix[i+2] = uint8((bl * 0xffff / a) >> 8)
			dst.Pix[i+3] = uint8((a / n) >> 8)
		}
	}
	return dst
}

// HasPictureExtension says whether name looks like a picture this can read.
func HasPictureExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}
