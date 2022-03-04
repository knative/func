package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

var hexs = []byte("0123456789abcdef")
var space = []byte(" ")
var newLine = []byte("\n")
var tab = []byte("\t")

// This program generates zz_filesystem_generated.go file containing byte array variable named templatesZip.
// The variable contains zip of "./templates" directory.
func main() {
	var zipBuff bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuff)
	err := filepath.Walk("templates", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Clean(path) == "templates" {
			return nil
		}

		name, err := filepath.Rel("templates", path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if info.IsDir() {
			name = name + "/"
		}

		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetMode(info.Mode())

		w, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			_, err = w.Write(b)
			if err != nil {
				return err
			}
		}

		return nil
	})
	zipWriter.Close()
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile("zz_filesystem_generated.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	srcOut := bufio.NewWriter(f)
	defer srcOut.Flush()

	_, err = fmt.Fprintf(srcOut, "// Code generated by go generate; DO NOT EDIT.\npackage function\n\nvar templatesZip = []byte{\n")
	if err != nil {
		log.Fatal(err)
	}

	buff := make([]byte, 32)
	hexDigitWithComma := []byte("0x00,")
	for {
		n, err := zipBuff.Read(buff)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				log.Fatal(err)
			}
		}

		_, err = srcOut.Write(tab)
		if err != nil {
			log.Fatal(err)
		}

		for i, b := range buff[:n] {
			hexDigitWithComma[2] = hexs[b>>4]
			hexDigitWithComma[3] = hexs[b&0x0f]
			_, err = srcOut.Write(hexDigitWithComma)
			if err != nil {
				log.Fatal(err)
			}
			if i < n-1 {
				_, err = srcOut.Write(space)
				if err != nil {
					log.Fatal(err)
				}
			}
		}

		_, err = srcOut.Write(newLine)
		if err != nil {
			log.Fatal(err)
		}

	}

	_, err = fmt.Fprint(srcOut, "}\n")
	if err != nil {
		log.Fatal(err)
	}
}
