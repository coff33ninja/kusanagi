package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

var (
	svgPath = flag.String("svg", "", "path to input SVG")
	icoPath = flag.String("ico", "", "path to output ICO")
)

func main() {
	flag.Parse()
	if *svgPath == "" || *icoPath == "" {
		fmt.Fprintln(os.Stderr, "usage: gen-iconstool -svg=<path> -ico=<path>")
		os.Exit(1)
	}

	svgData, err := os.ReadFile(*svgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read SVG: %v\n", err)
		os.Exit(1)
	}

	sizes := []int{16, 32, 48, 64, 256}
	var images []image.Image

	for _, s := range sizes {
		fmt.Printf("Rendering %dx%d ...\n", s, s)
		img, err := renderSVG(svgData, s, s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render %dx%d: %v\n", s, s, err)
			os.Exit(1)
		}
		pngPath := filepath.Join(filepath.Dir(*icoPath), fmt.Sprintf("app-%d.png", s))
		f, err := os.Create(pngPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", pngPath, err)
			os.Exit(1)
		}
		png.Encode(f, img)
		f.Close()
		fmt.Printf("  -> %s\n", pngPath)
		images = append(images, img)
	}

	f, err := os.Create(*icoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create ICO: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := writeICO(f, images); err != nil {
		fmt.Fprintf(os.Stderr, "write ICO: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ICO saved: %s\n", *icoPath)
}

func renderSVG(data []byte, w, h int) (image.Image, error) {
	svgIcon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse SVG: %w", err)
	}

	svgIcon.SetTarget(0, 0, float64(w), float64(h))

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	svgIcon.Draw(raster, 1.0)

	return img, nil
}

func writeICO(w io.Writer, images []image.Image) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to write")
	}

	hdr := make([]byte, 6)
	binary.LittleEndian.PutUint16(hdr[0:2], 0)
	binary.LittleEndian.PutUint16(hdr[2:4], 1)
	binary.LittleEndian.PutUint16(hdr[4:6], uint16(len(images)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}

	type entry struct {
		w, h   byte
		size   uint32
		offset uint32
	}

	var entries []entry
	var dataBuf bytes.Buffer
	dataOff := int64(6 + len(images)*16)

	for _, img := range images {
		b := img.Bounds()
		d := b.Dx()
		rgba := image.NewRGBA(b)
		draw.Draw(rgba, b, img, b.Min, draw.Src)

		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, rgba); err != nil {
			return err
		}
		pngBytes := pngBuf.Bytes()
		entries = append(entries, entry{
			w:      byte(d % 256),
			h:      byte(d % 256),
			size:   uint32(len(pngBytes)),
			offset: uint32(dataOff),
		})
		dataBuf.Write(pngBytes)
		dataOff += int64(len(pngBytes))
	}

	for _, e := range entries {
		buf := make([]byte, 16)
		buf[0] = e.w
		buf[1] = e.h
		buf[2] = 0
		buf[3] = 0
		binary.LittleEndian.PutUint16(buf[4:6], 1)
		binary.LittleEndian.PutUint16(buf[6:8], 32)
		binary.LittleEndian.PutUint32(buf[8:12], e.size)
		binary.LittleEndian.PutUint32(buf[12:16], e.offset)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	_, err := io.Copy(w, &dataBuf)
	return err
}
