package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundToInt(t *testing.T) {
	tests := []struct {
		in   float64
		want int
	}{
		{0, 0},
		{0.4, 0},
		{0.5, 1},
		{1.49, 1},
		{1.5, 2},
		{-9.6, -10},
		{-10.0, -10},
		{-0.5, -1},
	}
	for _, tt := range tests {
		if got := roundToInt(tt.in); got != tt.want {
			t.Errorf("roundToInt(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestWriteCSV(t *testing.T) {
	accumulators := map[string]*filterAccumulator{
		"H": {
			count:     3,
			durations: []float64{300, 300, 300},
			gains:     []float64{100, 100, 100},
			binnings:  []int{1, 1, 1},
			temps:     []float64{-10, -10, -9},     // mode -10
			fNumbers:  []float64{6, 6, 6},          // mode 6.00
			ambTemps:  []float64{11.0, 12.0, 13.0}, // mean 12.0
		},
		"L": {
			count:     2,
			durations: []float64{120, 120},
			gains:     []float64{100, 100},
			binnings:  []int{1, 1},
			temps:     []float64{-10, -10},
			// no fNumber / ambient temp -> those cells stay empty
		},
		"X": { // unmapped -> should be skipped with a warning
			count:     1,
			durations: []float64{60},
		},
	}
	filterMap := map[string]int{"H": 43627, "L": 33995}

	out := filepath.Join(t.TempDir(), "acquisition.csv")
	if err := writeCSV(accumulators, filterMap, out); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)

	wantHeader := "filter,number,duration,binning,gain,sensorCooling,fNumber,darks,flats,flatDarks,bias,temperature"
	// Rows are sorted by filter name: H before L. X is unmapped and omitted.
	// H carries fNumber (6.00) and an averaged ambient temperature (12.0); L
	// carries neither, so those cells are empty.
	want := wantHeader + "\n" +
		"43627,3,300.0000,1,100.00,-10,6.00,,,,,12.0\n" +
		"33995,2,120.0000,1,100.00,-10,,,,,,\n"
	if got != want {
		t.Errorf("CSV mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "X") {
		t.Error("unmapped filter X should not appear in the CSV")
	}
}

func TestWriteCSVOmitsMissingValues(t *testing.T) {
	// A filter with no duration/gain/binning/temp should yield empty cells.
	accumulators := map[string]*filterAccumulator{
		"L": {count: 5},
	}
	filterMap := map[string]int{"L": 33995}

	out := filepath.Join(t.TempDir(), "acquisition.csv")
	if err := writeCSV(accumulators, filterMap, out); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	data, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines: %q", len(lines), lines)
	}
	if lines[1] != "33995,5,,,,,,,,,," {
		t.Errorf("row = %q, want %q", lines[1], "33995,5,,,,,,,,,,")
	}
}

// TestScanAndWriteCSV exercises the full pipeline: synthetic frames on disk ->
// scanDirectory -> writeCSV.
func TestScanAndWriteCSV(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "n1/ha_0.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir, "n1/ha_1.fits", lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))
	writeFile(t, dir, "n2/o_0.xisf", makeXISF(map[string]string{
		"IMAGETYP": "'LIGHT'", "FILTER": "'O'", "EXPTIME": "600.0",
		"GAIN": "100", "CCD-TEMP": "-10.0", "XBINNING": "1",
	}))

	acc, err := scanDirectory(dir)
	if err != nil {
		t.Fatalf("scanDirectory: %v", err)
	}

	out := filepath.Join(t.TempDir(), "acquisition.csv")
	if err := writeCSV(acc, map[string]int{"H": 43627, "O": 43628}, out); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	data, _ := os.ReadFile(out)
	want := "filter,number,duration,binning,gain,sensorCooling,fNumber,darks,flats,flatDarks,bias,temperature\n" +
		"43627,2,300.0000,1,100.00,-10,,,,,,\n" +
		"43628,1,600.0000,1,100.00,-10,,,,,,\n"
	if string(data) != want {
		t.Errorf("pipeline CSV mismatch:\n got: %q\nwant: %q", string(data), want)
	}
}
