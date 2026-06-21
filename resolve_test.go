package main

import (
	"path/filepath"
	"testing"
)

func TestResolveLightsDir(t *testing.T) {
	t.Run("descends into lights subdir", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "lights/h1.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))
		writeFile(t, root, "darks/d1.fits", lightFITS("DARK", "D", 300, 78, -10, 1))

		got := resolveLightsDir(root)
		want := filepath.Join(root, "lights")
		if got != want {
			t.Errorf("resolveLightsDir(%q) = %q, want %q", root, got, want)
		}
	})

	t.Run("leaves a lights dir unchanged", func(t *testing.T) {
		root := t.TempDir()
		lights := writeDir(t, root, "lights")
		writeFile(t, lights, "h1.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))

		if got := resolveLightsDir(lights); got != lights {
			t.Errorf("resolveLightsDir(%q) = %q, want unchanged", lights, got)
		}
	})

	t.Run("no lights subdir returns dir unchanged", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "h1.fits", lightFITS("LIGHT", "H", 300, 78, -10, 1))
		if got := resolveLightsDir(root); got != root {
			t.Errorf("resolveLightsDir(%q) = %q, want unchanged", root, got)
		}
	})
}
