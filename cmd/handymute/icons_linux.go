//go:build linux

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
)

func writeTrayIcons() (dir string, err error) {
	dir, err = os.MkdirTemp("", "handymute-icons-")
	if err != nil {
		return "", err
	}
	if err := writePNG(filepath.Join(dir, "handymute.png"), micCanvas(micIdle)); err != nil {
		return "", err
	}
	if err := writePNG(filepath.Join(dir, "handymute-active.png"), withHalo(micCanvas(micGreen), micGreen)); err != nil {
		return "", err
	}
	return dir, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
