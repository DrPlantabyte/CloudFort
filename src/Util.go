package main

import (
	"archive/zip"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cheggaaa/pb"
)

func nameFromFile(f string) string {
	fname := filepath.Base(f)
	i := strings.LastIndex(fname, ".")
	if i > 0 {
		return fname[:i]
	} else {
		return fname
	}
}
func swapFileSuffix(fpath string, newSuffix string) string {
	parent := filepath.Dir(fpath)
	root := nameFromFile(fpath)
	fname := root + newSuffix
	return filepath.Join(parent, fname)
}

func fileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	if os.IsNotExist(err) {
		return false
	} else if err == nil {
		return true
	}
	return false
}

func hashFile(fpath string) (string, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func listFiles(dirPath string, suffix string) ([]string, error) {
	allFiles, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return make([]string, 0), err
	}
	filtered := make([]string, 0, len(allFiles))
	for _, fi := range allFiles {
		f := filepath.Join(dirPath, fi.Name())
		if !fi.IsDir() && strings.HasSuffix(f, suffix) {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}
func listDirs(dirPath string) ([]string, error) {
	allFiles, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return make([]string, 0), err
	}
	filtered := make([]string, 0, len(allFiles))
	for _, fi := range allFiles {
		f := filepath.Join(dirPath, fi.Name())
		if fi.IsDir() {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

func unzipFiles(zipPath string, zipRoot string, dirPath string, filterFunc func(string) bool) error {
	fmt.Printf("Extracting files from %s in %s to %s\n", zipRoot, zipPath, dirPath)
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	//
	for _, zf := range zr.File {
		zfPath := zf.Name
		if zf.FileInfo().IsDir() {
			// ignore directories, will create as needed
			continue
		}
		relPath, err := filepath.Rel(zipRoot, zfPath)
		if err != nil {
			return err
		}
		if filterFunc(relPath) {
			outPath := filepath.Join(dirPath, relPath)
			outDir := filepath.Dir(outPath)
			err = os.MkdirAll(outDir, 0777)
			if err != nil {
				return err
			}
			outFile, err := os.Create(outPath)
			if err != nil {
				return err
			}
			zfReader, err := zf.Open()
			if err != nil {
				return err
			}
			//
			_, err = io.Copy(outFile, zfReader)
			outFile.Close()
			if err != nil {
				return err
			}
			//
			err = zfReader.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func zipFiles(dirRoot string, files []string, destFile string) error {
	outFile, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	for _, f := range files {
		relPath, err := filepath.Rel(dirRoot, f)
		if err != nil {
			return err
		}
		fi, err := os.Stat(f)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			fin, err := ioutil.ReadFile(f)
			if err != nil {
				return err
			}
			fout, err := zw.Create(relPath)
			if err != nil {
				return err
			}
			_, err = fout.Write(fin)
			if err != nil {
				return err
			}
		}
	}
	err = zw.Close()
	if err != nil {
		return err
	}
	return nil
}
func scanDir(dirPath string) ([]string, error) {
	files, err := ioutil.ReadDir(dirPath)
	flist := make([]string, 0, len(files)) // make(type, initial size (nil-filled), initial capacity)
	if err != nil {
		return make([]string, 0), err
	}
	for _, f := range files {
		fpath := filepath.Join(dirPath, f.Name())
		fi, err := os.Stat(fpath)
		if err != nil {
			return make([]string, 0), err
		}
		if fi.IsDir() {
			subfiles, err := scanDir(fpath)
			if err != nil {
				return make([]string, 0), err
			}
			flist = append(flist, subfiles...)
		} else {
			flist = append(flist, fpath)
		}
	}
	return flist, nil
}

func deleteDir(dirPath string) error {
	return os.RemoveAll(dirPath)
}

func sendFile(r io.Reader, w io.Writer, numBytes int64, showProgBar bool) error {
	sizeBuffer := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBuffer, uint64(numBytes))
	_, err := w.Write(sizeBuffer)
	if err != nil {
		return err
	}
	var progBar *pb.ProgressBar
	if showProgBar {
		fSize := numBytes
		progBar = pb.New(int(fSize)).SetUnits(pb.MB).SetRefreshRate(time.Millisecond * 100)
		(*progBar).ShowSpeed = true
		progBar.Start()
		r = progBar.NewProxyReader(r)
	}
	count := int64(0)
	for count < numBytes {
		buffer := make([]byte, minInt(0x10000, (numBytes-count)))
		c, err := r.Read(buffer)
		if err != nil {
			return err
		}
		count += int64(c)
		//time.Sleep(time.Millisecond * 100) // uncomment to test slowconnection
		_, err = w.Write(buffer[:c])
		if err != nil {
			return err
		}
	}
	if showProgBar {
		progBar.Finish()
	}
	return nil
}
func recvFile(r io.Reader, w io.Writer, showProgBar bool) error {
	sizeBuffer := make([]byte, 8)
	_, err := r.Read(sizeBuffer)
	if err != nil {
		return err
	}
	numBytes := int64(binary.BigEndian.Uint64(sizeBuffer))
	var progBar *pb.ProgressBar
	if showProgBar {
		fSize := numBytes
		progBar = pb.New(int(fSize)).SetUnits(pb.MB).SetRefreshRate(time.Millisecond * 100)
		(*progBar).ShowSpeed = true
		progBar.Start()
		r = progBar.NewProxyReader(r)
	}
	count := int64(0)
	for count < numBytes {
		buffer := make([]byte, minInt(0x10000, (numBytes-count)))
		c, err := r.Read(buffer)
		if err != nil {
			return err
		}
		//fmt.Printf("\tread %d bytes\n", c)
		count += int64(c)
		//time.Sleep(time.Millisecond * 100) // uncomment to test slowconnection
		_, err = w.Write(buffer[:c])
		if err != nil {
			return err
		}
	}
	if showProgBar {
		progBar.Finish()
	}
	return nil
}

func minInt(a, b int64) int64 {
	if b < a {
		return b
	}
	return a
}

func filterStrings(arr []string, cond func(string) bool) []string {
	result := make([]string, 0, len(arr))
	for i := range arr {
		if cond(arr[i]) {
			result = append(result, arr[i])
		}
	}
	return result
}

func strToUtf8(s string) []byte {
	bb := make([]byte, 0, len(s)*2)
	ra := []rune(s)
	for len(ra) > 0 {
		bbb := make([]byte, 4, 4)
		size := utf8.EncodeRune(bbb, ra[0])
		bb = append(bb, bbb[:size]...)
		ra = ra[1:]
	}
	return bb
}
func utf8ToStr(bb []byte) string {
	ra := make([]rune, 0, len(bb))
	for len(bb) > 0 {
		r, size := utf8.DecodeRune(bb)
		ra = append(ra, r)
		bb = bb[size:]
	}
	return string(ra)
}
