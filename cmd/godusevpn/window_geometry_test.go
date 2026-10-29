package main

import "testing"

func TestWindowGeometrySmallWorkAreaAndDPI(t *testing.T) {
	preferred := windowGeometry{420, 760, 380, 640}
	for _, test := range []struct {
		name string
		area windowWorkArea
		want windowGeometry
	}{
		{"vm-1041x720-workarea", windowWorkArea{0, 0, 1041, 720, 96, 96}, windowGeometry{420, 704, 380, 640}},
		{"wide-default-stays-narrow", windowWorkArea{0, 0, 2560, 1400, 96, 96}, preferred},
		{"125-percent", windowWorkArea{0, 0, 1041, 720, 120, 120}, windowGeometry{420, 560, 380, 560}},
		{"150-percent", windowWorkArea{0, 0, 1041, 720, 144, 144}, windowGeometry{420, 464, 380, 464}},
		{"narrow-both-axes", windowWorkArea{0, 0, 320, 480, 96, 96}, windowGeometry{304, 464, 304, 464}},
		{"top-left-taskbars", windowWorkArea{48, 32, 1041, 752, 96, 96}, windowGeometry{420, 704, 380, 640}},
		{"negative-monitor-origin", windowWorkArea{-1920, -1080, 0, 0, 144, 144}, windowGeometry{420, 704, 380, 640}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := fitWindowGeometry(preferred, test.area)
			if !ok || got != test.want {
				t.Fatalf("geometry: %+v wanted %+v", got, test.want)
			}
		})
	}
}

func TestWindowGeometryNeverOutgrowsAnySyntheticWorkArea(t *testing.T) {
	for _, dpi := range []int{48, 96, 120, 144, 168, 192, 240, 288, 480, 768} {
		for _, width := range []int{32, 240, 320, 420, 800, 1041, 1233, 1920, 3840} {
			for _, height := range []int{32, 240, 480, 600, 720, 840, 1080, 2160} {
				area := windowWorkArea{-width, -height, 0, 0, dpi, dpi}
				got, ok := fitWindowGeometry(windowGeometry{420, 760, 380, 640}, area)
				if !ok || got.Width < got.MinWidth || got.Height < got.MinHeight || got.MinWidth < 1 || got.MinHeight < 1 || got.Width*dpi/96 > width || got.Height*dpi/96 > height {
					t.Fatal("window or minimum exceeds work area", dpi, width, height, got)
				}
				// Wails centers the physical rectangle within rcWork. Negative
				// monitor origins must not cause a coordinate-zero assumption.
				x := area.Left + (width-got.Width*dpi/96)/2
				y := area.Top + (height-got.Height*dpi/96)/2
				if x < area.Left || y < area.Top || x+got.Width*dpi/96 > area.Right || y+got.Height*dpi/96 > area.Bottom {
					t.Fatal("centered geometry outside work area")
				}
			}
		}
	}
}

func TestWindowGeometryRejectsUnknownAndMalformedInputs(t *testing.T) {
	preferred := windowGeometry{420, 760, 380, 640}
	for _, area := range []windowWorkArea{{}, {0, 0, 1041, 720, 0, 96}, {0, 0, 1041, 720, 96, 769}, {10, 0, 9, 720, 96, 96}, {0, 0, 1041, 0, 96, 96}, {-1000001, 0, 1000, 720, 96, 96}, {0, 0, 100001, 720, 96, 96}} {
		if _, ok := fitWindowGeometry(preferred, area); ok {
			t.Fatal("invalid geometry accepted")
		}
	}
	for _, invalid := range []windowGeometry{{}, {420, 760, 0, 640}, {420, -1, 380, 640}, {100001, 760, 380, 640}} {
		if _, ok := fitWindowGeometry(invalid, windowWorkArea{0, 0, 1041, 720, 96, 96}); ok {
			t.Fatal("invalid preferred geometry accepted")
		}
	}
}
