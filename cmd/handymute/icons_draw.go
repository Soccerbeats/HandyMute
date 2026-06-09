//go:build windows

package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Tray-icon colors.
var (
	micIdle  = color.RGBA{R: 230, G: 230, B: 235, A: 255} // neutral mic when not talking
	micGreen = color.RGBA{R: 52, G: 199, B: 89, A: 255}   // iOS green, while actively dictating
)

const iconSize = 44 // canvas px; leaves padding around the 32-ish mic for the glow halo

// micCanvas renders a microphone glyph in col on a transparent iconSize×iconSize canvas,
// anti-aliased via 4x supersampling.
func micCanvas(col color.RGBA) *image.RGBA {
	const ss = 4
	out := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for oy := 0; oy < iconSize; oy++ {
		for ox := 0; ox < iconSize; ox++ {
			cov := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(ox) + (float64(sx)+0.5)/ss
					fy := float64(oy) + (float64(sy)+0.5)/ss
					if micInside(fx, fy) {
						cov++
					}
				}
			}
			if cov > 0 {
				out.SetRGBA(ox, oy, color.RGBA{col.R, col.G, col.B, uint8(cov * 255 / (ss * ss))})
			}
		}
	}
	return out
}

func micInside(x, y float64) bool {
	if roundRect(x, y, 16, 7, 28, 25, 6) {
		return true
	}
	dx, dy := x-22, y-22
	if d := math.Hypot(dx, dy); d <= 11 && d >= 8 && dy >= -7 {
		return true
	}
	if roundRect(x, y, 21, 33, 23, 38, 1) {
		return true
	}
	if roundRect(x, y, 15, 37.5, 29, 40, 1.2) {
		return true
	}
	return false
}

func roundRect(px, py, x0, y0, x1, y1, r float64) bool {
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	cx := clampF(px, x0+r, x1-r)
	cy := clampF(py, y0+r, y1-r)
	return math.Hypot(px-cx, py-cy) <= r
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// withHalo returns a new image with a soft glow behind the opaque pixels of mic.
func withHalo(mic *image.RGBA, glowColor color.RGBA) *image.RGBA {
	b := mic.Bounds()
	res := image.NewRGBA(b)

	alphaAt := func(x, y int) float64 {
		if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
			return 0
		}
		return float64(mic.RGBAAt(x, y).A) / 255
	}

	const radius = 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var best float64
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					a := alphaAt(x+dx, y+dy)
					if a <= 0 {
						continue
					}
					fall := 1 - float64(dx*dx+dy*dy)/float64(radius*radius+1)
					if fall > 0 {
						if v := a * fall; v > best {
							best = v
						}
					}
				}
			}
			if best > 0 {
				res.SetRGBA(x, y, color.RGBA{glowColor.R, glowColor.G, glowColor.B, uint8(best * 230)})
			}
		}
	}

	draw.Draw(res, b, mic, b.Min, draw.Over)
	return res
}
