package main

import (
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

func TestScanDirectoryEmpty(t *testing.T) {
	acc, err := scanDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}
	if len(acc) != 0 {
		t.Errorf("expected no accumulators, got %v", acc)
	}
}
