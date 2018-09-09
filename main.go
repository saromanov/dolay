package main

import (
	"archive/tar"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
)

// Files defines type for tar headers
type Files []*tar.Header

// Implementation of sorting for headers

// Len returns length of header
func (s Files) Len() int {
	return len(s)
}

// Swap provides swaping of two headers
func (s Files) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less provides compare of two files
func (s Files) Less(i, j int) bool {
	if s[i].Size != s[j].Size {
		return s[i].Size > s[j].Size
	}
	return s[i].Name < s[j].Name
}

type ManifestItem struct {
	Config   string
	RepoTags []string
	Layers   []string
}

type History struct {
	EmptyLayer bool   `json:"empty_layer,omitempty"`
	CreatedBy  string `json:"created_by,omitempty"`
}
type Image struct {
	History []History `json:"history,omitempty"`
}

// Layer defines docker layer
type Layer struct {
	Files Files
	Size  uint64
}

const (
	humanizedWidth = 7
	manifest       = "manifest.json"
)

func removeEmptyLayers(h []History, old []History) []History {
	for _, action := range old {
		if !action.EmptyLayer {
			h = append(h, action)
		}
	}
	return h
}
func run() error {
	tarPath := flag.String("f", "-", "layer.tar path")
	maxFiles := flag.Int("n", 10, "max files")
	lineWidth := flag.Int("l", 100, "screen line width")
	flag.Parse()

	r, err := os.Open(*tarPath)
	if err != nil {
		return fmt.Errorf("unable to open file: %v", err)
	}
	defer r.Close()

	var manifests []ManifestItem
	var img Image
	layers := make(map[string]*Layer)
	archive := tar.NewReader(r)
	for {
		hdr, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch {
		case strings.HasSuffix(hdr.Name, "/layer.tar"):
			record := tar.NewReader(archive)

			var fs []*tar.Header
			var total uint64
			for {
				h, err := record.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				fi := h.FileInfo()
				if fi.IsDir() {
					continue
				}
				fs = append(fs, h)
				total += uint64(h.Size)
			}
			layers[hdr.Name] = &Layer{fs, total}

		case hdr.Name == manifest:
			if err := json.NewDecoder(archive).Decode(&manifests); err != nil {
				return err
			}
		case strings.HasSuffix(hdr.Name, ".json"):
			if err := json.NewDecoder(archive).Decode(&img); err != nil {
				return err
			}
		}
	}

	manifest := manifests[0]
	history := img.History[:0]
	history = removeEmptyLayers(history, img.History)

	cmdWidth := *lineWidth - humanizedWidth - 4
	for i, action := range history {
		layer := layers[manifest.Layers[i]]

		var cmd string
		tokens := strings.SplitN(action.CreatedBy, "/bin/sh -c ", 2)
		if len(tokens) == 2 {
			cmd = tokens[1]
		} else {
			cmd = action.CreatedBy
		}
		if len(cmd) > cmdWidth {
			cmd = cmd[:cmdWidth]
		}

		fmt.Println()
		fmt.Println(strings.Repeat("=", *lineWidth))
		color.Blue(humanizeBytes(layer.Size), "\t $", strings.Replace(cmd, "\t", " ", 0))
		fmt.Println(strings.Repeat("=", *lineWidth))
		sort.Sort(layer.Files)
		for j, f := range layer.Files {
			if j >= *maxFiles {
				break
			}
			fmt.Println(humanizeBytes(uint64(f.Size)), "\t", f.Name)
		}
	}

	return nil
}

func humanizeBytes(sz uint64) string {
	return pad(humanize.Bytes(sz), humanizedWidth)
}

func pad(s string, n int) string {
	return strings.Repeat(" ", n-len(s)) + s
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
