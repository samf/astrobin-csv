package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrameLight(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.fits",
		lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 2))

	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info == nil {
		t.Fatal("parseFrame returned nil for a valid light frame")
	}
	if info.filterName != "H" {
		t.Errorf("filterName = %q, want %q", info.filterName, "H")
	}
	if info.exptime == nil || *info.exptime != 300.0 {
		t.Errorf("exptime = %v, want 300", info.exptime)
	}
	if info.gain == nil || *info.gain != 100.0 {
		t.Errorf("gain = %v, want 100", info.gain)
	}
	if info.binning == nil || *info.binning != 2 {
		t.Errorf("binning = %v, want 2", info.binning)
	}
	if info.ccdTemp == nil || *info.ccdTemp != -10.0 {
		t.Errorf("ccdTemp = %v, want -10", info.ccdTemp)
	}
}

func TestParseFrameTemperatureAndFNumber(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.fits", makeFITS([]fitsKeyword{
		{"IMAGETYP", "'LIGHT'"},
		{"FILTER", "'H'"},
		{"EXPTIME", "300.0"},
		{"AMBTEMP", "11.31"},
		{"FOCRATIO", "6.0"},
	}))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info == nil {
		t.Fatal("parseFrame returned nil")
	}
	if info.ambTemp == nil || *info.ambTemp != 11.31 {
		t.Errorf("ambTemp = %v, want 11.31", info.ambTemp)
	}
	if info.fNumber == nil || *info.fNumber != 6.0 {
		t.Errorf("fNumber = %v, want 6.0", info.fNumber)
	}
}

func TestParseFrameSkipsNonLight(t *testing.T) {
	path := writeFile(t, t.TempDir(), "dark.fits",
		lightFITS("DARK", "H", 300.0, 100.0, -10.0, 1))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for DARK frame, got %+v", info)
	}
}

func TestParseFrameAcceptsLightFrameVariant(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.fits",
		lightFITS("LIGHT FRAME", "L", 120.0, 100.0, -10.0, 1))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil for 'LIGHT FRAME' variant")
	}
	if info.filterName != "L" {
		t.Errorf("filterName = %q, want %q", info.filterName, "L")
	}
}

func TestParseFrameMissingFilter(t *testing.T) {
	path := writeFile(t, t.TempDir(), "nofilter.fits", makeFITS([]fitsKeyword{
		{"IMAGETYP", "'LIGHT'"},
		{"EXPTIME", "300.0"},
	}))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil when FILTER is absent, got %+v", info)
	}
}

func TestParseFrameUnknownExtension(t *testing.T) {
	path := writeFile(t, t.TempDir(), "notes.txt", []byte("hello"))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for unknown extension, got %+v", info)
	}
}

func TestParseFrameExposureFallback(t *testing.T) {
	// No EXPTIME, only EXPOSURE -- should fall back to EXPOSURE.
	path := writeFile(t, t.TempDir(), "light.fits", makeFITS([]fitsKeyword{
		{"IMAGETYP", "'LIGHT'"},
		{"FILTER", "'R'"},
		{"EXPOSURE", "45.5"},
		{"SET-TEMP", "-5.0"},
	}))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info == nil || info.exptime == nil || *info.exptime != 45.5 {
		t.Fatalf("exptime fallback failed: %+v", info)
	}
	// CCD-TEMP absent, SET-TEMP present -> fall back to SET-TEMP.
	if info.ccdTemp == nil || *info.ccdTemp != -5.0 {
		t.Errorf("ccdTemp fallback = %v, want -5", info.ccdTemp)
	}
}

func TestParseFrameXISF(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.xisf", makeXISF(map[string]string{
		"IMAGETYP": "'LIGHT'",
		"FILTER":   "'O'",
		"EXPTIME":  "600.0",
		"GAIN":     "100",
		"CCD-TEMP": "-10.0",
		"XBINNING": "1",
	}))
	info, err := parseFrame(path)
	if err != nil {
		t.Fatalf("parseFrame: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil for XISF light frame")
	}
	if info.filterName != "O" {
		t.Errorf("filterName = %q, want %q", info.filterName, "O")
	}
	if info.exptime == nil || *info.exptime != 600.0 {
		t.Errorf("exptime = %v, want 600", info.exptime)
	}
}

func TestMostCommon(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, ok := mostCommon([]int{}); ok {
			t.Error("expected ok=false for empty slice")
		}
	})
	t.Run("clear winner", func(t *testing.T) {
		got, ok := mostCommon([]float64{120, 300, 300, 300})
		if !ok || got != 300 {
			t.Errorf("got (%v, %v), want (300, true)", got, ok)
		}
	})
	t.Run("tie breaks to first seen", func(t *testing.T) {
		// 120 and 300 each appear twice; 120 appears first.
		got, ok := mostCommon([]float64{120, 120, 300, 300})
		if !ok || got != 120 {
			t.Errorf("got (%v, %v), want (120, true)", got, ok)
		}
	})
	t.Run("single", func(t *testing.T) {
		got, ok := mostCommon([]int{2})
		if !ok || got != 2 {
			t.Errorf("got (%v, %v), want (2, true)", got, ok)
		}
	})
}

func TestMean(t *testing.T) {
	if _, ok := mean(nil); ok {
		t.Error("expected ok=false for empty slice")
	}
	if got, ok := mean([]float64{11.0, 12.0, 13.0}); !ok || got != 12.0 {
		t.Errorf("mean = (%v, %v), want (12, true)", got, ok)
	}
	if got, _ := mean([]float64{-10, -5}); got != -7.5 {
		t.Errorf("mean = %v, want -7.5", got)
	}
}

func TestScanDirectory(t *testing.T) {
	dir := t.TempDir()
	// Three Ha lights across two subdirectories.
	writeFile(t, dir, "n1/ha_0.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir, "n1/ha_1.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir, "n2/ha_2.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -9.0, 1))
	// One L light (XISF) and a dark that must be ignored.
	writeFile(t, dir, "n2/l_0.xisf", makeXISF(map[string]string{
		"IMAGETYP": "'LIGHT'", "FILTER": "'L'", "EXPTIME": "120.0",
		"GAIN": "100", "CCD-TEMP": "-10.0", "XBINNING": "1",
	}))
	writeFile(t, dir, "n2/dark.fits", lightFITS("DARK", "H", 300.0, 100.0, -10.0, 1))
	// A non-frame file that must be ignored.
	writeFile(t, dir, "n2/readme.txt", []byte("ignore me"))

	acc, err := scanDirectory(dir)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}

	if len(acc) != 2 {
		t.Fatalf("got %d filters, want 2 (H, L): %v", len(acc), acc)
	}
	if acc["H"].count != 3 {
		t.Errorf("H count = %d, want 3", acc["H"].count)
	}
	if acc["L"].count != 1 {
		t.Errorf("L count = %d, want 1", acc["L"].count)
	}
	if dur, _ := mostCommon(acc["H"].durations); dur != 300.0 {
		t.Errorf("H duration mode = %v, want 300", dur)
	}
	// Temps for H: -10, -10, -9 -> mode -10.
	if temp, _ := mostCommon(acc["H"].temps); temp != -10.0 {
		t.Errorf("H temp mode = %v, want -10", temp)
	}
}

func TestScanDirectoryFollowsSymlinks(t *testing.T) {
	// The real frames live outside the scanned directory; the scanned lights
	// directory reaches them only through a symlink -- both a symlinked
	// subdirectory and a symlinked individual file.
	store := t.TempDir()
	writeFile(t, store, "night1/ha_0.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, store, "night1/ha_1.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	loose := writeFile(t, store, "loose/l_0.fits", lightFITS("LIGHT", "L", 120.0, 100.0, -10.0, 1))

	lights := t.TempDir()
	if err := os.Symlink(filepath.Join(store, "night1"), filepath.Join(lights, "night1")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	if err := os.Symlink(loose, filepath.Join(lights, "l_0.fits")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}

	acc, err := scanDirectory(lights)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	if acc["H"] == nil || acc["H"].count != 2 {
		t.Errorf("H count = %v, want 2", acc["H"])
	}
	if acc["L"] == nil || acc["L"].count != 1 {
		t.Errorf("L count = %v, want 1", acc["L"])
	}
}

// TestMergeAccumulators exercises the multi-directory path: scanning two
// directories independently and summing them must behave as though the frames
// lived in one directory. In particular the per-frame stats are pooled, so the
// combined duration mode is recomputed over all frames rather than picked from
// either directory alone.
func TestMergeAccumulators(t *testing.T) {
	dir1 := t.TempDir()
	// dir1: H is mostly 300s (mode 300 on its own), plus one L.
	writeFile(t, dir1, "h0.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir1, "h1.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir1, "h2.fits", lightFITS("LIGHT", "H", 120.0, 100.0, -10.0, 1))
	writeFile(t, dir1, "l0.fits", lightFITS("LIGHT", "L", 120.0, 100.0, -10.0, 1))

	dir2 := t.TempDir()
	// dir2: H is all 120s (mode 120 on its own); enough to flip the combined mode.
	writeFile(t, dir2, "h0.fits", lightFITS("LIGHT", "H", 120.0, 100.0, -10.0, 1))
	writeFile(t, dir2, "h1.fits", lightFITS("LIGHT", "H", 120.0, 100.0, -10.0, 1))
	writeFile(t, dir2, "h2.fits", lightFITS("LIGHT", "H", 120.0, 100.0, -10.0, 1))

	acc1, err := scanDirectory(dir1)
	if err != nil {
		t.Fatalf("scanDirectory dir1: %v", err)
	}
	acc2, err := scanDirectory(dir2)
	if err != nil {
		t.Fatalf("scanDirectory dir2: %v", err)
	}

	combined := map[string]*filterAccumulator{}
	mergeAccumulators(combined, acc1)
	mergeAccumulators(combined, acc2)

	if len(combined) != 2 {
		t.Fatalf("got %d filters, want 2 (H, L): %v", len(combined), combined)
	}
	if combined["H"].count != 6 {
		t.Errorf("H count = %d, want 6 (3 + 3)", combined["H"].count)
	}
	if combined["L"].count != 1 {
		t.Errorf("L count = %d, want 1 (only in dir1)", combined["L"].count)
	}
	// Combined durations: 300x2, 120x4 -> mode 120, which matches neither the
	// dir1-only mode (300). Proves the stat slices are pooled, not pre-collapsed.
	if dur, _ := mostCommon(combined["H"].durations); dur != 120.0 {
		t.Errorf("H combined duration mode = %v, want 120", dur)
	}
}

// TestCalLibraryMerge checks that calibration frames from several directories
// are pooled before they're matched to filters.
func TestCalLibraryMerge(t *testing.T) {
	lib := &calLibrary{}
	lib.merge(&calLibrary{
		darks: []*frameInfo{calFrame(300, 78, -10, 1, "D", "DARK")},
		flats: []*frameInfo{calFrame(2.5, 78, -10, 1, "H", "FLAT")},
	})
	lib.merge(&calLibrary{
		darks:     []*frameInfo{calFrame(300, 78, -10, 1, "D", "DARK")},
		flatDarks: []*frameInfo{calFrame(2.5, 78, -10, 1, "H", "DARKFLAT")},
		bias:      []*frameInfo{calFrame(0, 78, -10, 1, "B", "BIAS")},
	})

	if len(lib.darks) != 2 {
		t.Errorf("darks = %d, want 2", len(lib.darks))
	}
	if len(lib.flats) != 1 {
		t.Errorf("flats = %d, want 1", len(lib.flats))
	}
	if len(lib.flatDarks) != 1 {
		t.Errorf("flatDarks = %d, want 1", len(lib.flatDarks))
	}
	if len(lib.bias) != 1 {
		t.Errorf("bias = %d, want 1", len(lib.bias))
	}
	if lib.total() != 5 {
		t.Errorf("total = %d, want 5", lib.total())
	}
}

func TestScanDirectoryEmpty(t *testing.T) {
	acc, err := scanDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	if len(acc) != 0 {
		t.Errorf("expected no accumulators, got %v", acc)
	}
}
