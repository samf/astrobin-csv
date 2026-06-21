package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fitsKeyword is one card's keyword and its already-formatted value field
// (e.g. "'LIGHT'" for a string value, or "300.0" for a number).
type fitsKeyword struct {
	keyword string
	value   string
}

// fitsCardImage pads (or truncates) s to a single 80-byte FITS card.
func fitsCardImage(s string) string {
	if len(s) > fitsCardSize {
		return s[:fitsCardSize]
	}
	return s + strings.Repeat(" ", fitsCardSize-len(s))
}

// makeFITS builds the bytes of a minimal FITS file: the given value cards
// followed by an END card, padded to a whole number of 2880-byte blocks.
func makeFITS(kws []fitsKeyword) []byte {
	var b strings.Builder
	for _, kw := range kws {
		b.WriteString(fitsCardImage(fmt.Sprintf("%-8s= %s", kw.keyword, kw.value)))
	}
	b.WriteString(fitsCardImage("END"))
	data := b.String()
	if pad := (fitsBlockSize - len(data)%fitsBlockSize) % fitsBlockSize; pad > 0 {
		data += strings.Repeat(" ", pad)
	}
	return []byte(data)
}

// lightFITS builds the bytes of a typical NINA light-frame FITS file.
func lightFITS(imagetyp, filter string, exptime, gain, temp float64, xbin int) []byte {
	return makeFITS([]fitsKeyword{
		{"SIMPLE", "T"},
		{"BITPIX", "8"},
		{"NAXIS", "0"},
		{"IMAGETYP", fmt.Sprintf("'%s'", imagetyp)},
		{"FILTER", fmt.Sprintf("'%s'", filter)},
		{"EXPTIME", fmt.Sprintf("%.6f", exptime)},
		{"GAIN", fmt.Sprintf("%.2f", gain)},
		{"CCD-TEMP", fmt.Sprintf("%.2f", temp)},
		{"XBINNING", fmt.Sprintf("%d", xbin)},
	})
}

// dwarfFITS builds the bytes of a Dwarf 3-style FITS file: no IMAGETYP keyword
// and an uncooled DET-TEMP instead of CCD-TEMP/SET-TEMP. A "" filter is written
// as an empty FILTER value (as the Dwarf does for darks).
func dwarfFITS(filter string, exptimeSec, gain, detTemp, xbin int) []byte {
	return makeFITS([]fitsKeyword{
		{"SIMPLE", "T"},
		{"NAXIS", "2"},
		{"EXPTIME", fmt.Sprintf("%d.", exptimeSec)}, // Dwarf writes e.g. "60."
		{"GAIN", fmt.Sprintf("%d", gain)},
		{"XBINNING", fmt.Sprintf("%d", xbin)},
		{"DET-TEMP", fmt.Sprintf("%d", detTemp)},
		{"FILTER", fmt.Sprintf("'%s'", filter)},
		{"CAMERA", "'TELE'"},
	})
}

// makeXISF builds the bytes of a monolithic XISF file whose XML header carries
// the given FITSKeyword name/value pairs.
func makeXISF(kws map[string]string) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<xisf version="1.0" xmlns="http://www.pixinsight.com/xisf">`)
	sb.WriteString(`<Image geometry="100:100:1" sampleFormat="UInt16">`)
	for name, value := range kws {
		sb.WriteString(fmt.Sprintf(`<FITSKeyword name="%s" value="%s" comment=""/>`, name, value))
	}
	sb.WriteString(`</Image></xisf>`)
	xmlBytes := []byte(sb.String())

	out := make([]byte, 0, 16+len(xmlBytes))
	out = append(out, []byte("XISF0100")...)
	var lengthAndReserved [8]byte
	binary.LittleEndian.PutUint32(lengthAndReserved[0:4], uint32(len(xmlBytes)))
	out = append(out, lengthAndReserved[:]...)
	out = append(out, xmlBytes...)
	return out
}

// writeDir creates directory name inside dir and returns its full path.
func writeDir(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
	return path
}

// writeFile writes content to name inside dir, creating parent directories,
// and returns the full path. It fails the test on error.
func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return path
}
