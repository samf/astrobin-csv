package main

import "testing"

func TestReadFITSHeader(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.fits",
		lightFITS("LIGHT", "H", 300.0, 100.0, -10.0, 1))

	header, err := readFITSHeader(path)
	if err != nil {
		t.Fatalf("readFITSHeader: %v", err)
	}

	want := map[string]string{
		"SIMPLE":   "T",
		"IMAGETYP": "LIGHT", // quotes stripped
		"FILTER":   "H",
		"EXPTIME":  "300.000000",
		"GAIN":     "100.00",
		"CCD-TEMP": "-10.00",
		"XBINNING": "1",
	}
	for k, v := range want {
		if got := header[k]; got != v {
			t.Errorf("header[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestReadFITSHeaderTruncated(t *testing.T) {
	// A complete 2880-byte block with no END card, and nothing more to read,
	// should error (on the next ReadFull) rather than loop forever.
	block := []byte(fitsCardImage("SIMPLE  = T"))
	for len(block) < fitsBlockSize {
		block = append(block, ' ')
	}
	path := writeFile(t, t.TempDir(), "bad.fits", block)

	if _, err := readFITSHeader(path); err == nil {
		t.Fatal("expected error reading END-less FITS header, got nil")
	}
}

func TestParseFITSValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"string", "'LIGHT   '          / image type", "LIGHT"},
		{"string with spaces inside", "'LIGHT FRAME'", "LIGHT FRAME"},
		{"numeric with comment", "   300.0 / exposure time", "300.0"},
		{"numeric no comment", "100", "100"},
		{"logical", "T / simple", "T"},
		{"negative float", "-10.00", "-10.00"},
		{"unterminated quote", "'oops", "oops"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFITSValue([]byte(tt.in)); got != tt.want {
				t.Errorf("parseFITSValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadXISFHeader(t *testing.T) {
	path := writeFile(t, t.TempDir(), "light.xisf", makeXISF(map[string]string{
		"IMAGETYP": "'LIGHT'", // single-quoted, should be stripped
		"FILTER":   "'O'",
		"EXPTIME":  "600.0", // unquoted numeric, kept as-is
		"GAIN":     "100",
		"CCD-TEMP": "-10.0",
		"XBINNING": "1",
	}))

	header, err := readXISFHeader(path)
	if err != nil {
		t.Fatalf("readXISFHeader: %v", err)
	}

	want := map[string]string{
		"IMAGETYP": "LIGHT",
		"FILTER":   "O",
		"EXPTIME":  "600.0",
		"GAIN":     "100",
		"CCD-TEMP": "-10.0",
		"XBINNING": "1",
	}
	for k, v := range want {
		if got := header[k]; got != v {
			t.Errorf("header[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestReadXISFHeaderBadSignature(t *testing.T) {
	path := writeFile(t, t.TempDir(), "notxisf.xisf", []byte("NOTXISF!and some more bytes"))
	if _, err := readXISFHeader(path); err == nil {
		t.Fatal("expected error for bad XISF signature, got nil")
	}
}
