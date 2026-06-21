package main

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// calLibrary holds the calibration frames discovered alongside the lights,
// bucketed by frame type.
type calLibrary struct {
	darks     []*frameInfo
	flats     []*frameInfo
	flatDarks []*frameInfo
	bias      []*frameInfo
}

func (l *calLibrary) total() int {
	return len(l.darks) + len(l.flats) + len(l.flatDarks) + len(l.bias)
}

// classifyCalibration maps an IMAGETYP string to one of our calibration bucket
// names, or "" for frames we don't count (lights and PixInsight masters).
func classifyCalibration(imagetyp string) string {
	t := strings.ToUpper(imagetyp)
	// "Master Dark"/"Master Light" etc. are stacked integrations, not
	// individual subs -- never count them.
	if strings.Contains(t, "MASTER") || strings.Contains(t, "LIGHT") {
		return ""
	}
	flat := strings.Contains(t, "FLAT")
	dark := strings.Contains(t, "DARK")
	switch {
	case flat && dark: // DARKFLAT / "FLAT DARK"
		return "flatdark"
	case flat:
		return "flat"
	case strings.Contains(t, "BIAS"):
		return "bias"
	case dark:
		return "dark"
	}
	return ""
}

// scanCalibration searches the sibling directories of lightsDir (i.e. the other
// directories under its parent) for calibration frames and buckets them by
// type. Light frames and master integrations found there are ignored, so the
// PixInsight output directories (calibrated/, registered/, master/, ...) don't
// pollute the counts.
func scanCalibration(lightsDir string) (*calLibrary, error) {
	lib := &calLibrary{}

	lightsAbs, err := filepath.Abs(lightsDir)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(lightsAbs)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", parent, err)
	}

	var calFiles []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sibling := filepath.Join(parent, e.Name())
		if sibling == lightsAbs {
			continue // that's the lights directory itself
		}
		_ = filepath.WalkDir(sibling, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // tolerate unreadable subtrees
			}
			if d.IsDir() {
				return nil
			}
			suffix := strings.ToLower(filepath.Ext(p))
			if fitsExtensions[suffix] || xisfExtensions[suffix] {
				calFiles = append(calFiles, p)
			}
			return nil
		})
	}

	if len(calFiles) == 0 {
		return lib, nil
	}
	sort.Strings(calFiles)
	fmt.Printf("Scanning %d files in sibling directories for calibration frames...\n", len(calFiles))

	for _, p := range calFiles {
		header, ok, err := readHeaderFor(p)
		if !ok || err != nil {
			continue
		}
		info := extractFrame(header)
		switch classifyCalibration(info.imagetyp) {
		case "dark":
			lib.darks = append(lib.darks, info)
		case "flat":
			lib.flats = append(lib.flats, info)
		case "flatdark":
			lib.flatDarks = append(lib.flatDarks, info)
		case "bias":
			lib.bias = append(lib.bias, info)
		}
	}
	return lib, nil
}

// darkKey identifies the exposure/gain/temperature/binning combination a dark
// frame matches. Exposure is held in tenths of a second and temperature is
// rounded so the sensor's actual temperature (e.g. -9.9) matches its setpoint
// (-10).
type darkKey struct {
	expDeci int
	gain    int
	temp    int
	binning int
}

// biasKey is like darkKey but exposure-independent (bias frames are ~0s).
type biasKey struct {
	gain    int
	temp    int
	binning int
}

func deciSeconds(s float64) int { return int(math.Round(s * 10)) }

func darkKeyOf(f *frameInfo) (darkKey, bool) {
	if f.exptime == nil || f.gain == nil || f.ccdTemp == nil || f.binning == nil {
		return darkKey{}, false
	}
	return darkKey{deciSeconds(*f.exptime), roundToInt(*f.gain), roundToInt(*f.ccdTemp), *f.binning}, true
}

func biasKeyOf(f *frameInfo) (biasKey, bool) {
	if f.gain == nil || f.ccdTemp == nil || f.binning == nil {
		return biasKey{}, false
	}
	return biasKey{roundToInt(*f.gain), roundToInt(*f.ccdTemp), *f.binning}, true
}

// countsFor returns the calibration-frame counts attributable to one light
// filter. Darks are matched to the filter's representative exposure/gain/temp/
// binning; bias by gain/temp/binning (exposure-independent); flats and
// flat-darks by filter name.
func (l *calLibrary) countsFor(filterName string, acc *filterAccumulator) (darks, flats, flatDarks, bias int) {
	expF, okE := mostCommon(acc.durations)
	gainF, okG := mostCommon(acc.gains)
	tempF, okT := mostCommon(acc.temps)
	binF, okB := mostCommon(acc.binnings)

	if okE && okG && okT && okB {
		want := darkKey{deciSeconds(expF), roundToInt(gainF), roundToInt(tempF), binF}
		for _, d := range l.darks {
			if k, ok := darkKeyOf(d); ok && k == want {
				darks++
			}
		}
	}
	if okG && okT && okB {
		want := biasKey{roundToInt(gainF), roundToInt(tempF), binF}
		for _, b := range l.bias {
			if k, ok := biasKeyOf(b); ok && k == want {
				bias++
			}
		}
	}
	for _, f := range l.flats {
		if f.filterName == filterName {
			flats++
		}
	}
	for _, fd := range l.flatDarks {
		if fd.filterName == filterName {
			flatDarks++
		}
	}
	return darks, flats, flatDarks, bias
}
