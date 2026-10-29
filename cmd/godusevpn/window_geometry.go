package main

// Window geometry is expressed in Wails' 96-DPI logical units. The work-area
// input is physical pixels, excluding taskbars and other reserved desktop UI.
// This file is pure: tests never query a monitor or change display settings.
type windowWorkArea struct {
	Left, Top, Right, Bottom int
	DPIX, DPIY               int
}

type windowGeometry struct {
	Width, Height, MinWidth, MinHeight int
}

func fitWindowGeometry(preferred windowGeometry, area windowWorkArea) (windowGeometry, bool) {
	if area.DPIX < 48 || area.DPIX > 768 || area.DPIY < 48 || area.DPIY > 768 ||
		preferred.Width <= 0 || preferred.Height <= 0 || preferred.MinWidth <= 0 || preferred.MinHeight <= 0 ||
		preferred.Width > 100000 || preferred.Height > 100000 || preferred.MinWidth > 100000 || preferred.MinHeight > 100000 {
		return windowGeometry{}, false
	}
	// Bound coordinates before subtraction; Win32 RECT uses signed 32-bit
	// coordinates and secondary monitors may have a negative origin.
	for _, coordinate := range []int{area.Left, area.Top, area.Right, area.Bottom} {
		if coordinate < -1000000 || coordinate > 1000000 {
			return windowGeometry{}, false
		}
	}
	width, height := int64(area.Right)-int64(area.Left), int64(area.Bottom)-int64(area.Top)
	if width <= 0 || height <= 0 || width > 100000 || height > 100000 {
		return windowGeometry{}, false
	}
	// Leave up to 8 logical pixels on each edge. On unusually tiny work areas
	// reduce only the margin, never force a minimum bigger than actual space.
	available := func(pixels int64, dpi int) int {
		minimumPixel := (int64(dpi) + 95) / 96
		if pixels < minimumPixel {
			return 0
		}
		margin := (int64(8)*int64(dpi) + 95) / 96
		margin = min(margin, (pixels-minimumPixel)/2)
		return int((pixels - 2*margin) * 96 / int64(dpi))
	}
	maxWidth, maxHeight := available(width, area.DPIX), available(height, area.DPIY)
	if maxWidth < 1 || maxHeight < 1 {
		return windowGeometry{}, false
	}
	result := windowGeometry{
		MinWidth: min(preferred.MinWidth, maxWidth), MinHeight: min(preferred.MinHeight, maxHeight),
		Width: min(preferred.Width, maxWidth), Height: min(preferred.Height, maxHeight),
	}
	result.Width, result.Height = max(result.Width, result.MinWidth), max(result.Height, result.MinHeight)
	return result, true
}
