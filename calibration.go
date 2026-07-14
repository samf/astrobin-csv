package main

import (
	"fmt"
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

// merge folds other's calibration frames into l, so calibration discovered
// alongside several lights directories is pooled before matching to filters.
func (l *calLibrary) merge(other *calLibrary) {
	l.darks = append(l.darks, other.darks...)
	l.flats = append(l.flats, other.flats...)
	l.flatDarks = append(l.flatDarks, other.flatDarks...)
	l.bias = append(l.bias, other.bias...)
}

// classifyCalibration decides which calibration bucket a frame belongs to,
// returning "" for frames we don't count (lights and PixInsight masters). It
// prefers the IMAGETYP keyword, but falls back to the name of the directory the
// frame was found in when IMAGETYP is absent -- some cameras (e.g. the Dwarf 3)
// write no IMAGETYP, so the user's darks/, flats/, ... directories are the only
// signal.
func classifyCalibration(imagetyp, dirName string) string {
	t := strings.ToUpper(imagetyp)
	// "Master Dark"/"Master Light" etc. are stacked integrations, not
	// individual subs -- never count them.
	if strings.Contains(t, "MASTER") || strings.Contains(t, "LIGHT") {
		return ""
	}
	if c := calTypeFromName(t); c != "" {
		return c
	}
	// No usable IMAGETYP -- fall back to the directory name, but never treat a
	// stray lights directory as calibration.
	d := strings.ToUpper(dirName)
	if strings.Contains(d, "LIGHT") {
		return ""
	}
	return calTypeFromName(d)
}

// calTypeFromName classifies an already-uppercased string (an IMAGETYP value or
// a directory name) into a calibration bucket.
func calTypeFromName(s string) string {
	flat := strings.Contains(s, "FLAT")
	dark := strings.Contains(s, "DARK")
	switch {
	case flat && dark: // DARKFLAT / "FLAT DARK" / "darkflats"
		return "flatdark"
	case flat:
		return "flat"
	case strings.Contains(s, "BIAS"):
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

	// Each candidate carries the name of the top-level sibling directory it came
	// from, so we can classify by directory when IMAGETYP is missing.
	type candidate struct{ path, dir string }
	var candidates []candidate
	for _, e := range entries {
		sibling := filepath.Join(parent, e.Name())
		if !isDir(sibling) { // follows symlinks, so symlinked siblings count
			continue
		}
		if sibling == lightsAbs {
			continue // that's the lights directory itself
		}
		dirName := e.Name()
		_ = walkFiles(sibling, func(p string) {
			suffix := strings.ToLower(filepath.Ext(p))
			if fitsExtensions[suffix] || xisfExtensions[suffix] {
				candidates = append(candidates, candidate{p, dirName})
			}
		})
	}

	if len(candidates) == 0 {
		return lib, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	fmt.Printf("Scanning %d files in sibling directories for calibration frames...\n", len(candidates))

	for _, c := range candidates {
		header, ok, err := readHeaderFor(c.path)
		if !ok || err != nil {
			continue
		}
		info := extractFrame(header)
		switch classifyCalibration(info.imagetyp, c.dir) {
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

// darkKey identifies the exposure/gain/binning combination a dark frame
// matches. Exposure is held in tenths of a second. Temperature is deliberately
// excluded: cooled cameras hold a stable setpoint (so it adds no discrimination),
// while uncooled cameras (e.g. the Dwarf 3) take darks at a different ambient
// temperature than the lights, so matching on it would reject every dark.
type darkKey struct {
	expDeci int
	gain    int
	binning int
}

// biasKey is like darkKey but exposure-independent (bias frames are ~0s).
type biasKey struct {
	gain    int
	binning int
}

func deciSeconds(s float64) int { return int(math.Round(s * 10)) }

func darkKeyOf(f *frameInfo) (darkKey, bool) {
	if f.exptime == nil || f.gain == nil || f.binning == nil {
		return darkKey{}, false
	}
	return darkKey{deciSeconds(*f.exptime), roundToInt(*f.gain), *f.binning}, true
}

func biasKeyOf(f *frameInfo) (biasKey, bool) {
	if f.gain == nil || f.binning == nil {
		return biasKey{}, false
	}
	return biasKey{roundToInt(*f.gain), *f.binning}, true
}

// countsFor returns the calibration-frame counts attributable to one light
// filter. Darks are matched to the filter's representative exposure/gain/
// binning; bias by gain/binning (exposure-independent); flats and flat-darks by
// filter name.
func (l *calLibrary) countsFor(filterName string, acc *filterAccumulator) (darks, flats, flatDarks, bias int) {
	expF, okE := mostCommon(acc.durations)
	gainF, okG := mostCommon(acc.gains)
	binF, okB := mostCommon(acc.binnings)

	if okE && okG && okB {
		want := darkKey{deciSeconds(expF), roundToInt(gainF), binF}
		for _, d := range l.darks {
			if k, ok := darkKeyOf(d); ok && k == want {
				darks++
			}
		}
	}
	if okG && okB {
		want := biasKey{roundToInt(gainF), binF}
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
