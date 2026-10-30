package server

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strconv"
	"strings"
)

// icon serves the home-screen icon, a sofa drawn here so there's no image
// file to keep in sync: /icons/192.png, /icons/512.png, /icons/180.png
// (Apple's size).
func icon(w http.ResponseWriter, r *http.Request) {
	size, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("file"), ".png"))
	if err != nil || size < 16 || size > 1024 {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	png.Encode(&buf, sofa(size))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(buf.Bytes())
}

// sofa draws a flat sofa in the dashboard's blue accent on its graphite
// background. Shapes are in fractions of the size, so it scales cleanly.
func sofa(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0x11, 0x12, 0x14, 0xff}), image.Point{}, draw.Src)
	accent := image.NewUniform(color.RGBA{0x7a, 0xa2, 0xff, 0xff})
	dark := image.NewUniform(color.RGBA{0x4f, 0x7a, 0xe6, 0xff})
	rect := func(x0, y0, x1, y1 float64, c image.Image) {
		f := float64(size)
		draw.Draw(img, image.Rect(int(x0*f), int(y0*f), int(x1*f), int(y1*f)), c, image.Point{}, draw.Src)
	}
	rect(0.24, 0.30, 0.76, 0.52, accent) // back
	rect(0.24, 0.50, 0.76, 0.64, dark)   // seat
	rect(0.15, 0.42, 0.27, 0.68, accent) // left arm
	rect(0.73, 0.42, 0.85, 0.68, accent) // right arm
	rect(0.20, 0.68, 0.25, 0.74, dark)   // legs
	rect(0.75, 0.68, 0.80, 0.74, dark)
	return img
}
