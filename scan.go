package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// frameInfo holds the relevant header values extracted from a single frame.
type frameInfo struct {
	filterName string
	exptime    *float64
	gain       *float64
	binning    *int
	ccdTemp    *float64 // sensor temperature (CCD-TEMP / SET-TEMP)
	ambTemp    *float64 // ambient air temperature (AMBTEMP)
	fNumber    *float64 // focal ratio (FOCRATIO)
	imagetyp   string
}

// filterAccumulator aggregates the per-frame stats for one filter.
type filterAccumulator struct {
	count     int
	durations []float64
	gains     []float64
	binnings  []int
	temps     []float64 // sensor temperatures
	ambTemps  []float64 // ambient air temperatures
	fNumbers  []float64 // focal ratios
}

// parseFrame reads a frame's header and extracts the fields we care about.
// It returns nil (with no error) for files that aren't light frames or that
// lack a usable filter name.
func parseFrame(path string) (*frameInfo, error) {
	suffix := strings.ToLower(filepath.Ext(path))

	var header map[string]string
	var err error
	switch {
	case fitsExtensions[suffix]:
		header, err = readFITSHeader(path)
	case xisfExtensions[suffix]:
		header, err = readXISFHeader(path)
	default:
		return nil, nil
	}
	if err != nil {
		fmt.Printf("  [skip] Could not read header from %s: %v\n", filepath.Base(path), err)
		return nil, nil
	}

	imagetyp := strings.ToUpper(strings.TrimSpace(header["IMAGETYP"]))
	// NINA usually writes "LIGHT" but accept "LIGHT FRAME" variants too.
	if !strings.Contains(imagetyp, "LIGHT") {
		return nil, nil
	}

	filterName := strings.TrimSpace(header["FILTER"])
	if filterName == "" {
		fmt.Printf("  [skip] No FILTER keyword in %s\n", filepath.Base(path))
		return nil, nil
	}

	exptime := headerFloat(header, "EXPTIME")
	if exptime == nil {
		exptime = headerFloat(header, "EXPOSURE")
	}
	gain := headerFloat(header, "GAIN")
	ccdTemp := headerFloat(header, "CCD-TEMP")
	if ccdTemp == nil {
		ccdTemp = headerFloat(header, "SET-TEMP")
	}
	ambTemp := headerFloat(header, "AMBTEMP")
	fNumber := headerFloat(header, "FOCRATIO")

	var binning *int
	if b := headerFloat(header, "XBINNING"); b != nil {
		v := int(*b)
		binning = &v
	}

	return &frameInfo{
		filterName: filterName,
		exptime:    exptime,
		gain:       gain,
		binning:    binning,
		ccdTemp:    ccdTemp,
		ambTemp:    ambTemp,
		fNumber:    fNumber,
		imagetyp:   imagetyp,
	}, nil
}

// headerFloat parses a header value as a float, returning nil if absent or
// unparseable.
func headerFloat(header map[string]string, key string) *float64 {
	v, ok := header[key]
	if !ok {
		return nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

// scanDirectory walks the directory tree and accumulates stats per filter.
func scanDirectory(root string) (map[string]*filterAccumulator, error) {
	accumulators := map[string]*filterAccumulator{}

	var allFiles []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		suffix := strings.ToLower(filepath.Ext(path))
		if fitsExtensions[suffix] || xisfExtensions[suffix] {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", root, err)
	}

	if len(allFiles) == 0 {
		fmt.Printf("No FITS or XISF files found under %s\n", root)
		return accumulators, nil
	}

	sort.Strings(allFiles)
	fmt.Printf("Found %d FITS/XISF files. Reading headers...\n", len(allFiles))

	for i, path := range allFiles {
		info, err := parseFrame(path)
		if err != nil {
			return nil, err
		}
		if info == nil {
			continue
		}

		acc := accumulators[info.filterName]
		if acc == nil {
			acc = &filterAccumulator{}
			accumulators[info.filterName] = acc
		}
		acc.count++
		if info.exptime != nil {
			acc.durations = append(acc.durations, *info.exptime)
		}
		if info.gain != nil {
			acc.gains = append(acc.gains, *info.gain)
		}
		if info.binning != nil {
			acc.binnings = append(acc.binnings, *info.binning)
		}
		if info.ccdTemp != nil {
			acc.temps = append(acc.temps, *info.ccdTemp)
		}
		if info.ambTemp != nil {
			acc.ambTemps = append(acc.ambTemps, *info.ambTemp)
		}
		if info.fNumber != nil {
			acc.fNumbers = append(acc.fNumbers, *info.fNumber)
		}

		if (i+1)%200 == 0 {
			fmt.Printf("  ...%d/%d files processed\n", i+1, len(allFiles))
		}
	}

	return accumulators, nil
}

// mostCommon returns the most frequent value in values, breaking ties in favor
// of the value that appears earliest. The boolean is false when values is
// empty.
func mostCommon[T comparable](values []T) (T, bool) {
	var zero T
	if len(values) == 0 {
		return zero, false
	}
	counts := make(map[T]int, len(values))
	for _, v := range values {
		counts[v]++
	}
	best := zero
	bestCount := -1
	for _, v := range values {
		if counts[v] > bestCount {
			best = v
			bestCount = counts[v]
		}
	}
	return best, true
}

// mean returns the arithmetic mean of values. The boolean is false when values
// is empty. Used for continuous quantities (e.g. ambient temperature) where a
// representative average is more meaningful than a mode.
func mean(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values)), true
}
