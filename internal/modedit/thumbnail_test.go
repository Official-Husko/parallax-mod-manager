package modedit

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeImage(t *testing.T, name string, encode func(*os.File) error) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encode(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the result is not a PNG: %v", err)
	}
	return img
}

func TestASmallPNGIsKeptAsItIs(t *testing.T) {
	src := writeImage(t, "small.png", func(f *os.File) error { return png.Encode(f, solid(200, 100, color.NRGBA{200, 50, 50, 255})) })
	th, err := PrepareThumbnail(src)
	if err != nil {
		t.Fatal(err)
	}
	if th.Resized || th.Width != 200 || th.Height != 100 || th.SourceWidth != 200 {
		t.Errorf("thumbnail = %+v", th)
	}
	img := decodePNG(t, th.PNG)
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("size = %v", b)
	}
	if r, g, b, _ := img.At(10, 10).RGBA(); r>>8 != 200 || g>>8 != 50 || b>>8 != 50 {
		t.Errorf("colour changed: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestALargePictureIsScaledDownKeepingItsProportions(t *testing.T) {
	for _, tc := range []struct{ w, h, wantW, wantH int }{
		{1024, 512, 512, 256},
		{600, 1200, 256, 512}, // portrait: the height is the long side
		{2000, 2000, 512, 512},
		{1500, 100, 512, 34},
	} {
		src := writeImage(t, "big.png", func(f *os.File) error { return png.Encode(f, solid(tc.w, tc.h, color.NRGBA{10, 120, 200, 255})) })
		th, err := PrepareThumbnail(src)
		if err != nil {
			t.Fatal(err)
		}
		if !th.Resized || th.Width != tc.wantW || th.Height != tc.wantH || th.SourceWidth != tc.w || th.SourceHeight != tc.h {
			t.Errorf("%dx%d -> %+v, want %dx%d", tc.w, tc.h, th, tc.wantW, tc.wantH)
		}
		if b := decodePNG(t, th.PNG).Bounds(); b.Dx() != tc.wantW || b.Dy() != tc.wantH {
			t.Errorf("%dx%d: encoded size %v", tc.w, tc.h, b)
		}
	}
}

func TestAPictureIsNeverScaledUp(t *testing.T) {
	src := writeImage(t, "tiny.png", func(f *os.File) error { return png.Encode(f, solid(64, 32, color.NRGBA{1, 2, 3, 255})) })
	th, err := PrepareThumbnail(src)
	if err != nil {
		t.Fatal(err)
	}
	if th.Resized || th.Width != 64 || th.Height != 32 {
		t.Errorf("thumbnail = %+v", th)
	}
}

func TestScalingAveragesColoursAndKeepsTransparency(t *testing.T) {
	// A 1024x1024 picture, left half black, right half white: scaled 2:1, the edge pixels stay pure
	// and the picture keeps its two halves.
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			if x < 512 {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
			}
		}
	}
	src := writeImage(t, "halves.png", func(f *os.File) error { return png.Encode(f, img) })
	th, err := PrepareThumbnail(src)
	if err != nil {
		t.Fatal(err)
	}
	out := decodePNG(t, th.PNG)
	if r, _, _, _ := out.At(10, 200).RGBA(); r>>8 > 5 {
		t.Errorf("left half is %d, want black", r>>8)
	}
	if r, _, _, _ := out.At(500, 200).RGBA(); r>>8 < 250 {
		t.Errorf("right half is %d, want white", r>>8)
	}

	// Transparent stays transparent; opaque stays opaque and is not darkened by the see-through part.
	tr := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 512; x < 1024; x++ {
			tr.SetNRGBA(x, y, color.NRGBA{200, 100, 50, 255})
		}
	}
	src = writeImage(t, "alpha.png", func(f *os.File) error { return png.Encode(f, tr) })
	th, err = PrepareThumbnail(src)
	if err != nil {
		t.Fatal(err)
	}
	out = decodePNG(t, th.PNG)
	if _, _, _, a := out.At(10, 100).RGBA(); a != 0 {
		t.Errorf("the transparent half has alpha %d", a>>8)
	}
	if r, g, b, a := out.At(400, 100).RGBA(); a>>8 != 255 || r>>8 < 195 || r>>8 > 205 || g>>8 < 95 || g>>8 > 105 || b>>8 < 45 || b>>8 > 55 {
		t.Errorf("the opaque half is %d %d %d alpha %d, want about 200 100 50 / 255", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestJPEGAndGIFAreAcceptedAndComeOutAsPNG(t *testing.T) {
	jpg := writeImage(t, "photo.jpg", func(f *os.File) error { return jpeg.Encode(f, solid(300, 200, color.NRGBA{90, 90, 200, 255}), nil) })
	th, err := PrepareThumbnail(jpg)
	if err != nil {
		t.Fatalf("jpeg: %v", err)
	}
	if b := decodePNG(t, th.PNG).Bounds(); b.Dx() != 300 || b.Dy() != 200 {
		t.Errorf("jpeg size %v", b)
	}

	g := writeImage(t, "anim.gif", func(f *os.File) error {
		pal := color.Palette{color.Black, color.White}
		frame := image.NewPaletted(image.Rect(0, 0, 40, 40), pal)
		return gif.Encode(f, frame, nil)
	})
	th, err = PrepareThumbnail(g)
	if err != nil {
		t.Fatalf("gif: %v", err)
	}
	if b := decodePNG(t, th.PNG).Bounds(); b.Dx() != 40 || b.Dy() != 40 {
		t.Errorf("gif size %v", b)
	}
}

func TestWhatIsNotAUsablePictureIsRefused(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for name, path := range map[string]string{
		"text renamed to png": write("fake.png", "this is not a picture"),
		"empty":               write("empty.png", ""),
		"a folder":            dir,
		"missing":             filepath.Join(dir, "nope.png"),
	} {
		if _, err := PrepareThumbnail(path); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAnAbsurdlyLargePictureIsRefusedBeforeItIsDecoded(t *testing.T) {
	// A valid PNG header that claims 20000 x 20000 pixels: refused from its size alone.
	var buf bytes.Buffer
	small := solid(1, 1, color.NRGBA{0, 0, 0, 255})
	png.Encode(&buf, small)
	data := buf.Bytes()
	// IHDR width and height are bytes 16-23 of a PNG.
	data[16], data[17], data[18], data[19] = 0, 0, 0x4e, 0x20
	data[20], data[21], data[22], data[23] = 0, 0, 0x4e, 0x20
	// The header chunk's checksum covers its type and data (bytes 12-28) and sits right after them.
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	p := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareThumbnail(p)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want a too-large refusal", err)
	}
}

func TestABigResultGetsAWarning(t *testing.T) {
	// Noise does not compress: a 512x512 noise picture encodes to well over 1 MB.
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	seed := uint32(12345)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = byte(seed >> 24)
	}
	src := writeImage(t, "noise.png", func(f *os.File) error { return png.Encode(f, img) })
	th, err := PrepareThumbnail(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(th.PNG) <= warnThumbnailBytes || len(th.Warnings) != 1 || !strings.Contains(th.Warnings[0], "1 MB") {
		t.Errorf("%d bytes, warnings %v; want a 1 MB warning", len(th.PNG), th.Warnings)
	}
	small := writeImage(t, "flat.png", func(f *os.File) error { return png.Encode(f, solid(64, 64, color.NRGBA{1, 1, 1, 255})) })
	if th, _ := PrepareThumbnail(small); len(th.Warnings) != 0 {
		t.Errorf("a small picture warned: %v", th.Warnings)
	}
}

func TestPictureExtensions(t *testing.T) {
	for name, want := range map[string]bool{"a.png": true, "a.JPG": true, "a.jpeg": true, "a.gif": true, "a.dds": false, "a.webp": false, "a": false} {
		if HasPictureExtension(name) != want {
			t.Errorf("HasPictureExtension(%q) = %v", name, !want)
		}
	}
}
