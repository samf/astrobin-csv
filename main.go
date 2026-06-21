// Command astrobin-csv scans a directory of LIGHT frames (FITS or XISF)
// captured by N.I.N.A., groups them by filter, and writes a CSV in the format
// AstroBin's "import acquisitions from CSV" dialogue expects:
//
//	https://welcome.astrobin.com/importing-acquisitions-from-csv
//
// One row is written per filter, aggregating every light frame found for that
// filter across all nights/sessions under the given directory.
//
// Filter name -> AstroBin numeric filter ID mapping is read from a small YAML
// config file (default: ~/.astrobin-csv.yaml).
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
)

// CSV column order, per the official AstroBin import spec. (The date column is
// omitted deliberately -- we aggregate across all nights into one row per
// filter, so a single date isn't meaningful.)
var csvFields = []string{
	"filter",
	"number",
	"duration",
	"binning",
	"gain",
	"sensorCooling",
	"fNumber",
	"darks",
	"flats",
	"flatDarks",
	"bias",
	"temperature",
}

var fitsExtensions = map[string]bool{".fits": true, ".fit": true, ".fts": true}
var xisfExtensions = map[string]bool{".xisf": true}

// CLI is the command-line interface, parsed by kong.
var CLI struct {
	Directory     string `arg:"" name:"directory" type:"existingdir" help:"Directory containing light frames (searched recursively)."`
	Output        string `name:"output" short:"o" type:"path" default:"acquisition.csv" help:"Output CSV path."`
	Config        string `name:"config" short:"c" type:"path" default:"~/.astrobin-csv.yaml" help:"YAML filter-name -> AstroBin-filter-ID config."`
	NoCalibration bool   `name:"no-calibration" help:"Don't search sibling directories for dark/flat/bias/flat-dark frames."`
}

func main() {
	kong.Parse(&CLI,
		kong.Name("astrobin-csv"),
		kong.Description("Generate an AstroBin acquisition CSV from NINA FITS/XISF light frames."),
		kong.UsageOnError(),
	)

	filterMap, err := loadFilterMap(CLI.Config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	accumulators, err := scanDirectory(CLI.Directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if len(accumulators) == 0 {
		fmt.Println("No light frames found. Nothing to write.")
		os.Exit(1)
	}

	fmt.Println("\nSummary by filter:")
	for _, name := range sortedKeys(accumulators) {
		acc := accumulators[name]
		dur, ok := mostCommon(acc.durations)
		durStr := "?"
		totalHours := 0.0
		if ok {
			durStr = fmt.Sprintf("%.0fs", dur)
			totalHours = float64(acc.count) * dur / 3600
		}
		fmt.Printf("  %-10s %4d frames  x %6s  (~%.2fh)\n", name, acc.count, durStr, totalHours)
	}

	var calibration *calLibrary
	if !CLI.NoCalibration {
		calibration, err = scanCalibration(CLI.Directory)
		if err != nil {
			// Calibration is best-effort; warn but still write the CSV.
			fmt.Fprintf(os.Stderr, "warning: could not scan for calibration frames: %v\n", err)
		} else if calibration.total() > 0 {
			fmt.Printf("Calibration frames found: %d darks, %d flats, %d flat-darks, %d bias\n",
				len(calibration.darks), len(calibration.flats), len(calibration.flatDarks), len(calibration.bias))
		}
	}

	if err := writeCSV(accumulators, filterMap, calibration, CLI.Output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// sortedKeys returns the keys of a filter-accumulator map in sorted order.
func sortedKeys(m map[string]*filterAccumulator) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeCSV writes the aggregated per-filter rows to outputPath. calibration may
// be nil, in which case the dark/flat/flatDark/bias columns are left empty.
func writeCSV(accumulators map[string]*filterAccumulator, filterMap map[string]int, calibration *calLibrary, outputPath string) error {
	lines := []string{strings.Join(csvFields, ",")}

	var unmapped []string
	rows := 0
	for _, filterName := range sortedKeys(accumulators) {
		acc := accumulators[filterName]

		astrobinID, ok := filterMap[filterName]
		if !ok {
			unmapped = append(unmapped, filterName)
			continue
		}

		row := map[string]string{
			"filter":        fmt.Sprintf("%d", astrobinID),
			"number":        fmt.Sprintf("%d", acc.count),
			"duration":      "",
			"binning":       "",
			"gain":          "",
			"sensorCooling": "",
			"fNumber":       "",
			"darks":         "",
			"flats":         "",
			"flatDarks":     "",
			"bias":          "",
			"temperature":   "",
		}
		if duration, ok := mostCommon(acc.durations); ok {
			row["duration"] = fmt.Sprintf("%.4f", duration)
		}
		if binning, ok := mostCommon(acc.binnings); ok {
			row["binning"] = fmt.Sprintf("%d", binning)
		}
		if gain, ok := mostCommon(acc.gains); ok {
			row["gain"] = fmt.Sprintf("%.2f", gain)
		}
		if temp, ok := mostCommon(acc.temps); ok {
			row["sensorCooling"] = fmt.Sprintf("%d", roundToInt(temp))
		}
		// Focal ratio is a fixed optical property, so the mode is right.
		if fNumber, ok := mostCommon(acc.fNumbers); ok {
			row["fNumber"] = fmt.Sprintf("%.2f", fNumber)
		}
		// Ambient temperature drifts through the night; report the average.
		if ambTemp, ok := mean(acc.ambTemps); ok {
			row["temperature"] = fmt.Sprintf("%.1f", ambTemp)
		}
		if calibration != nil {
			darks, flats, flatDarks, bias := calibration.countsFor(filterName, acc)
			if darks > 0 {
				row["darks"] = fmt.Sprintf("%d", darks)
			}
			if flats > 0 {
				row["flats"] = fmt.Sprintf("%d", flats)
			}
			if flatDarks > 0 {
				row["flatDarks"] = fmt.Sprintf("%d", flatDarks)
			}
			if bias > 0 {
				row["bias"] = fmt.Sprintf("%d", bias)
			}
		}

		cells := make([]string, len(csvFields))
		for i, field := range csvFields {
			cells[i] = row[field]
		}
		lines = append(lines, strings.Join(cells, ","))
		rows++
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Printf("\nWrote %s (%d filter rows)\n", outputPath, rows)

	if len(unmapped) > 0 {
		fmt.Println("\nWARNING: these filter names had no entry in the filter config and were skipped:")
		for _, name := range unmapped {
			fmt.Printf("  - %s\n", name)
		}
		fmt.Println("Add them to your filter config and re-run.")
	}
	return nil
}

// roundToInt rounds a float to the nearest integer (half away from zero).
func roundToInt(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}
