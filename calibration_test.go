package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyCalibration(t *testing.T) {
	tests := []struct {
		imagetyp, dir, want string
	}{
		// IMAGETYP is authoritative when present.
		{"LIGHT", "lights", ""},
		{"DARK", "darks", "dark"},
		{"FLAT", "flats", "flat"},
		{"BIAS", "bias", "bias"},
		{"DARKFLAT", "whatever", "flatdark"},
		{"FLAT DARK", "whatever", "flatdark"},
		{"Master Dark", "darks", ""},   // a stacked master, not a sub
		{"Master Light", "lights", ""}, // ditto
		{"Master Flat", "flats", ""},
		// No IMAGETYP (e.g. Dwarf 3) -- fall back to the directory name.
		{"", "darks", "dark"},
		{"", "flats", "flat"},
		{"", "bias", "bias"},
		{"", "darkflats", "flatdark"},
		{"", "lights", ""}, // a stray lights dir is never calibration
		{"", "unknown", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := classifyCalibration(tt.imagetyp, tt.dir); got != tt.want {
			t.Errorf("classifyCalibration(%q, %q) = %q, want %q", tt.imagetyp, tt.dir, got, tt.want)
		}
	}
}

// calFrame is a small constructor for synthetic calibration frames in tests.
func calFrame(exp, gain, temp float64, bin int, filter, typ string) *frameInfo {
	e, g, tp, b := exp, gain, temp, bin
	return &frameInfo{
		filterName: filter,
		exptime:    &e,
		gain:       &g,
		ccdTemp:    &tp,
		binning:    &b,
		imagetyp:   typ,
	}
}

func TestCalibrationCountsFor(t *testing.T) {
	lib := &calLibrary{
		darks: []*frameInfo{
			calFrame(300, 78, -10.0, 1, "D", "DARK"),
			calFrame(300, 78, 5.0, 1, "D", "DARK"),  // different temp still matches (temp ignored)
			calFrame(60, 78, -10.0, 1, "D", "DARK"), // wrong exposure for a 300s filter
		},
		flats: []*frameInfo{
			calFrame(2.5, 78, -10, 1, "H", "FLAT"),
			calFrame(2.5, 78, -10, 1, "O", "FLAT"),
		},
		flatDarks: []*frameInfo{
			calFrame(2.5, 78, -10, 1, "H", "DARKFLAT"),
		},
		bias: []*frameInfo{
			calFrame(0, 78, -10, 1, "B", "BIAS"),
		},
	}
	// H: 300s lights, gain 78, sensor -10, bin 1.
	acc := &filterAccumulator{
		durations: []float64{300, 300},
		gains:     []float64{78, 78},
		temps:     []float64{-10, -10},
		binnings:  []int{1, 1},
	}

	darks, flats, flatDarks, bias := lib.countsFor("H", acc)
	if darks != 2 {
		t.Errorf("darks = %d, want 2 (both 300s darks regardless of temp; 60s excluded)", darks)
	}
	if flats != 1 {
		t.Errorf("flats = %d, want 1 (only the H flat)", flats)
	}
	if flatDarks != 1 {
		t.Errorf("flatDarks = %d, want 1 (H)", flatDarks)
	}
	if bias != 1 {
		t.Errorf("bias = %d, want 1 (matched by gain/temp/bin)", bias)
	}
}

func TestScanCalibration(t *testing.T) {
	root := t.TempDir()
	// The lights directory itself must be ignored by the calibration scan.
	writeFile(t, root, "lights/l1.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))
	// Raw darks at two exposures.
	writeFile(t, root, "darks/d1.fits", lightFITS("DARK", "D", 300, 78, -10, 1))
	writeFile(t, root, "darks/d2.fits", lightFITS("DARK", "D", 300, 78, -9.9, 1))
	writeFile(t, root, "darks/d3.fits", lightFITS("DARK", "D", 60, 78, -10, 1))
	// Flats for H.
	writeFile(t, root, "flats/f1.fits", lightFITS("FLAT", "H", 2.5, 78, -10, 1))
	writeFile(t, root, "flats/f2.fits", lightFITS("FLAT", "H", 2.5, 78, -10, 1))
	// A PixInsight master dark that must NOT be counted.
	writeFile(t, root, "master/md.fits", makeFITS([]fitsKeyword{
		{"IMAGETYP", "'Master Dark'"},
		{"EXPOSURE", "300.0"},
		{"GAIN", "78"},
		{"CCD-TEMP", "-10.0"},
		{"XBINNING", "1"},
	}))
	// A processed-light copy (as PixInsight would emit) must be ignored too.
	writeFile(t, root, "registered/r1.xisf", makeXISF(map[string]string{
		"IMAGETYP": "'LIGHT'", "FILTER": "'H'", "EXPTIME": "300.0",
		"GAIN": "78", "CCD-TEMP": "-10.0", "XBINNING": "1",
	}))

	lib, err := scanCalibration(filepath.Join(root, "lights"))
	if err != nil {
		t.Fatalf("scanCalibration: %v", err)
	}
	if len(lib.darks) != 3 {
		t.Errorf("darks = %d, want 3", len(lib.darks))
	}
	if len(lib.flats) != 2 {
		t.Errorf("flats = %d, want 2", len(lib.flats))
	}
	if len(lib.bias) != 0 || len(lib.flatDarks) != 0 {
		t.Errorf("bias=%d flatDarks=%d, want 0/0", len(lib.bias), len(lib.flatDarks))
	}
}

// TestDwarfPipeline covers a Dwarf 3-style dataset: frames carry no IMAGETYP,
// so lights are detected by absence of a type and darks are classified by their
// directory name. Darks are shot at a different (uncooled) temperature than the
// lights, so the match must ignore temperature.
func TestDwarfPipeline(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "lights/l1.fits", dwarfFITS("Astro", 60, 60, 27, 1))
	writeFile(t, root, "lights/l2.fits", dwarfFITS("Astro", 60, 60, 28, 1))
	writeFile(t, root, "darks/d1.fits", dwarfFITS("", 60, 60, 39, 1)) // hotter, no filter
	writeFile(t, root, "darks/d2.fits", dwarfFITS("", 60, 60, 40, 1))
	writeFile(t, root, "darks/d3.fits", dwarfFITS("", 60, 60, 38, 1))
	writeFile(t, root, "darks/d4.fits", dwarfFITS("", 30, 60, 39, 1)) // wrong exposure

	acc, err := scanDirectory(filepath.Join(root, "lights"))
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	if acc["Astro"] == nil || acc["Astro"].count != 2 {
		t.Fatalf("expected 2 'Astro' lights, got %+v", acc)
	}

	lib, err := scanCalibration(filepath.Join(root, "lights"))
	if err != nil {
		t.Fatalf("scanCalibration: %v", err)
	}
	if len(lib.darks) != 4 {
		t.Fatalf("expected 4 darks discovered (by dir name), got %d", len(lib.darks))
	}

	out := filepath.Join(t.TempDir(), "acquisition.csv")
	if err := writeCSV(acc, map[string]int{"Astro": 9999}, lib, out); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	data, _ := os.ReadFile(out)
	// 3 darks match the 60s exposure (the 30s one excluded); no cooling temp
	// (DET-TEMP isn't sensorCooling) and no fNumber/ambient -> empty cells.
	want := "9999,2,60.0000,1,60.00,,,3,,,,\n"
	if !strings.Contains(string(data), want) {
		t.Errorf("expected Dwarf row %q in:\n%s", want, string(data))
	}
}

// TestScanWithCalibration exercises the full pipeline including calibration
// attribution and CSV output.
func TestScanWithCalibration(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "lights/h1.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))
	writeFile(t, root, "lights/h2.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))
	writeFile(t, root, "darks/d1.fits", lightFITS("DARK", "D", 300, 78, -10, 1))
	writeFile(t, root, "darks/d2.fits", lightFITS("DARK", "D", 300, 78, -9.9, 1))
	writeFile(t, root, "darks/d3.fits", lightFITS("DARK", "D", 60, 78, -10, 1)) // unused

	acc, err := scanDirectory(filepath.Join(root, "lights"))
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	lib, err := scanCalibration(filepath.Join(root, "lights"))
	if err != nil {
		t.Fatalf("scanCalibration: %v", err)
	}

	out := filepath.Join(t.TempDir(), "acquisition.csv")
	if err := writeCSV(acc, map[string]int{"H": 43627}, lib, out); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	data, _ := os.ReadFile(out)
	// darks column (index 7) should be 2 for the H row; the 60s dark is excluded.
	want := "43627,2,300.0000,1,78.00,-10,,2,,,,\n"
	if !strings.Contains(string(data), want) {
		t.Errorf("expected H row %q in:\n%s", want, string(data))
	}
}
