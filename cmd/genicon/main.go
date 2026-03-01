// cmd/genicon generates a printer + shooting star icon as multi-size ICO.
// Output: pkg/tray/icon.ico and wix/assets/icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))

	// Transparent background
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	drawPrinter(img)
	drawShootingStar(img)

	sizes := []int{256, 48, 32, 16}
	ico := encodeICO(img, sizes)

	targets := []string{
		filepath.Join("pkg", "tray", "icon.ico"),
		filepath.Join("wix", "assets", "icon.ico"),
	}
	for _, t := range targets {
		if err := os.MkdirAll(filepath.Dir(t), 0755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(t, ico, 0644); err != nil {
			panic(err)
		}
	}
}

// --- Drawing ---

var (
	printerBody  = color.RGBA{60, 60, 68, 255}    // dark gray
	printerFront = color.RGBA{80, 80, 90, 255}     // lighter gray
	paperColor   = color.RGBA{245, 245, 245, 255}  // off-white
	trayColor    = color.RGBA{45, 45, 52, 255}      // darker
	accent       = color.RGBA{0, 140, 255, 255}     // blue accent
	starYellow   = color.RGBA{255, 220, 50, 255}    // gold
	trailColor   = color.RGBA{255, 180, 50, 200}    // orange trail
)

func drawPrinter(img *image.RGBA) {
	w, h := 256, 256

	// Paper tray (input) – top portion
	fillRect(img, 68, 50, 188, 75, paperColor)
	// Lines on paper
	for y := 56; y < 72; y += 5 {
		fillRect(img, 80, y, 176, y+2, color.RGBA{200, 200, 210, 255})
	}

	// Printer body – main block
	fillRoundedRect(img, 40, 75, w-40, 175, 12, printerBody)

	// Front panel (lighter shade)
	fillRect(img, 48, 130, w-48, 170, printerFront)

	// Paper output slot
	fillRect(img, 60, 165, w-60, 172, trayColor)

	// Paper coming out
	drawOutputPaper(img, 75, 168, 180, 230)

	// Status LED
	fillCircle(img, 75, 105, 5, color.RGBA{0, 220, 80, 255})

	// Blue accent strip across front
	fillRect(img, 48, 125, w-48, 131, accent)

	// Paper tray (output) at bottom
	fillRect(img, 55, h-32, w-55, h-25, trayColor)

	// Buttons on front panel
	fillCircle(img, w-75, 148, 6, color.RGBA{100, 100, 110, 255})
	fillCircle(img, w-95, 148, 6, color.RGBA{100, 100, 110, 255})
}

func drawOutputPaper(img *image.RGBA, x0, y0, x1, y1 int) {
	// Paper with a slight curl effect
	for y := y0; y < y1; y++ {
		progress := float64(y-y0) / float64(y1-y0)
		// Slight curve outward at bottom
		offset := int(progress * progress * 6)
		lx := x0 - offset
		rx := x1 + offset
		for x := lx; x < rx; x++ {
			if x >= 0 && x < 256 && y >= 0 && y < 256 {
				// Slight shadow at edges
				if x == lx || x == rx-1 {
					img.Set(x, y, color.RGBA{220, 220, 225, 255})
				} else {
					img.Set(x, y, paperColor)
				}
			}
		}
	}
	// Text lines on output paper
	for y := y0 + 8; y < y1-10; y += 6 {
		lineEnd := x1 - 20
		if (y-y0)/6%3 == 2 {
			lineEnd = x0 + 60
		}
		fillRect(img, x0+10, y, lineEnd, y+2, color.RGBA{190, 190, 200, 255})
	}
}

func drawShootingStar(img *image.RGBA) {
	// Star position: upper-right area
	cx, cy := 210, 40

	// Draw trail first (behind star)
	drawTrail(img, cx, cy)

	// Draw star burst
	drawStar(img, cx, cy, 22, 9, 5, starYellow)

	// Inner bright center
	drawStar(img, cx, cy, 12, 5, 5, color.RGBA{255, 255, 200, 255})

	// Tiny sparkle
	fillCircle(img, cx, cy, 3, color.RGBA{255, 255, 255, 255})
}

func drawTrail(img *image.RGBA, starX, starY int) {
	// Diagonal trail going down-left from the star
	length := 90
	angle := math.Pi * 0.75 // 135 degrees (down-left)

	for i := 0; i < length; i++ {
		t := float64(i) / float64(length)
		x := float64(starX) + math.Cos(angle)*float64(i)
		y := float64(starY) + math.Sin(angle)*float64(i)

		// Trail gets thinner and more transparent further from star
		width := int(6.0 * (1.0 - t*0.8))
		alpha := uint8(200.0 * (1.0 - t))

		tc := color.RGBA{trailColor.R, trailColor.G, trailColor.B, alpha}

		for dy := -width; dy <= width; dy++ {
			px, py := int(x), int(y)+dy
			if px >= 0 && px < 256 && py >= 0 && py < 256 {
				blendPixel(img, px, py, tc)
			}
		}
	}

	// Secondary thin trail
	for i := 0; i < length-20; i++ {
		t := float64(i) / float64(length)
		x := float64(starX) + math.Cos(angle+0.15)*float64(i)
		y := float64(starY) + math.Sin(angle+0.15)*float64(i)
		alpha := uint8(120.0 * (1.0 - t))
		tc := color.RGBA{255, 240, 100, alpha}
		px, py := int(x), int(y)
		if px >= 0 && px < 256 && py >= 0 && py < 256 {
			blendPixel(img, px, py, tc)
		}
	}
}

func drawStar(img *image.RGBA, cx, cy, outerR, innerR, points int, c color.Color) {
	totalPoints := points * 2
	for angle := 0.0; angle < 360.0; angle += 0.5 {
		rad := angle * math.Pi / 180.0
		// Determine if this angle lands on an outer or inner point
		segAngle := 360.0 / float64(totalPoints)
		idx := int(angle / segAngle)
		frac := (angle - float64(idx)*segAngle) / segAngle

		var r1, r2 float64
		if idx%2 == 0 {
			r1 = float64(outerR)
			r2 = float64(innerR)
		} else {
			r1 = float64(innerR)
			r2 = float64(outerR)
		}
		radius := r1 + (r2-r1)*frac

		for r := 0.0; r < radius; r += 0.5 {
			px := cx + int(math.Cos(rad-math.Pi/2)*r)
			py := cy + int(math.Sin(rad-math.Pi/2)*r)
			if px >= 0 && px < 256 && py >= 0 && py < 256 {
				img.Set(px, py, c)
			}
		}
	}
}

// --- Primitives ---

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x >= 0 && x < 256 && y >= 0 && y < 256 {
				img.Set(x, y, c)
			}
		}
	}
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, r int, c color.Color) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x >= 0 && x < 256 && y >= 0 && y < 256 {
				// Check corners
				inCorner := false
				var cx2, cy2 int
				if x < x0+r && y < y0+r {
					cx2, cy2 = x0+r, y0+r
					inCorner = true
				} else if x >= x1-r && y < y0+r {
					cx2, cy2 = x1-r, y0+r
					inCorner = true
				} else if x < x0+r && y >= y1-r {
					cx2, cy2 = x0+r, y1-r
					inCorner = true
				} else if x >= x1-r && y >= y1-r {
					cx2, cy2 = x1-r, y1-r
					inCorner = true
				}
				if inCorner {
					dx := float64(x - cx2)
					dy := float64(y - cy2)
					if dx*dx+dy*dy > float64(r*r) {
						continue
					}
				}
				img.Set(x, y, c)
			}
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx := float64(x - cx)
			dy := float64(y - cy)
			if dx*dx+dy*dy <= float64(r*r) {
				if x >= 0 && x < 256 && y >= 0 && y < 256 {
					img.Set(x, y, c)
				}
			}
		}
	}
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	existing := img.RGBAAt(x, y)
	alpha := float64(c.A) / 255.0
	inv := 1.0 - alpha
	blended := color.RGBA{
		R: uint8(float64(c.R)*alpha + float64(existing.R)*inv),
		G: uint8(float64(c.G)*alpha + float64(existing.G)*inv),
		B: uint8(float64(c.B)*alpha + float64(existing.B)*inv),
		A: uint8(math.Min(255, float64(c.A)+float64(existing.A)*inv)),
	}
	img.Set(x, y, blended)
}

// --- ICO Encoding ---

func encodeICO(src *image.RGBA, sizes []int) []byte {
	type icoEntry struct {
		data   []byte
		width  int
		height int
	}

	entries := make([]icoEntry, len(sizes))
	for i, size := range sizes {
		resized := resize(src, size)
		var buf bytes.Buffer
		png.Encode(&buf, resized)
		entries[i] = icoEntry{data: buf.Bytes(), width: size, height: size}
	}

	// ICO header: 6 bytes
	// Each directory entry: 16 bytes
	headerSize := 6 + 16*len(entries)
	var out bytes.Buffer

	// ICONDIR header
	binary.Write(&out, binary.LittleEndian, uint16(0))              // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1))              // type: icon
	binary.Write(&out, binary.LittleEndian, uint16(len(entries)))   // count

	// Calculate offsets
	offset := uint32(headerSize)
	for _, e := range entries {
		w := uint8(e.width)
		h := uint8(e.height)
		if e.width == 256 {
			w = 0 // 0 means 256 in ICO format
		}
		if e.height == 256 {
			h = 0
		}
		out.WriteByte(w)                                                      // width
		out.WriteByte(h)                                                      // height
		out.WriteByte(0)                                                      // color palette
		out.WriteByte(0)                                                      // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))                    // color planes
		binary.Write(&out, binary.LittleEndian, uint16(32))                   // bits per pixel
		binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))          // size
		binary.Write(&out, binary.LittleEndian, offset)                       // offset
		offset += uint32(len(e.data))
	}

	// Image data
	for _, e := range entries {
		out.Write(e.data)
	}

	return out.Bytes()
}

// resize scales src to size×size using nearest-neighbor.
func resize(src *image.RGBA, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := x * srcW / size
			sy := y * srcH / size
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
