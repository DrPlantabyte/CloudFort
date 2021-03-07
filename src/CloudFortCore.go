package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
)

const CloudFortVersion = "1.0.0"

const (
	STATUS_AVAILABLE   = "available"
	STATUS_DOWNLOADING = "downloading"
	STATUS_CHECKOUT    = "checked-out"
)

const (
	COM_CONCHECK = "DWARF"
	COM_STATUS   = "status"
	COM_CHECKOUT = "checkout"
	COM_CHECKIN  = "checkin"
	COM_RELEASE  = "release"
)

const (
	RESP_CONCHECK = "FORTRESS"
	RESP_DOWNLOAD = "download"
	RESP_UPLOAD   = "upload"
	RESP_ERROR    = "error"
	RESP_SUCCESS  = "success"
)

type LockToken struct {
	Status          string
	Expires         string
	CurrentOverseer string
	MagicRunes      string // validation hash generated unique for each check-out to prevent the wrong world from being checked in
}

var saveRegexes []*regexp.Regexp

func init() {
	saveRegexes = SaveFileRegexes()
}

func SaveFileRegexes() []*regexp.Regexp {
	regexStrs := []string{
		`raw[/\\]graphics[/\\].*`,
		`raw[/\\]objects[/\\].*\.txt`,
		`art_image-\d*\.dat`,
		`feature-\d*\.dat`,
		`region_snapshot-\d*\.dat`,
		`site-\d*\.dat`,
		`unit-\d*\.dat`,
		`world\.dat`,
		`world\.sav`,
	}
	matchers := make([]*regexp.Regexp, 0, len(regexStrs))
	for _, s := range regexStrs {
		m := regexp.MustCompile(s)
		matchers = append(matchers, m)
	}
	return matchers
}

// returns the root folder with world.dat|.sav
// this function is needed because users may zip the whole folder or just the contents
func findSaveZipRoot(zipPath string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		n := filepath.Base(f.Name)
		if n == "world.dat" || n == "world.sav" {
			// in root of save
			root := filepath.Dir(f.Name)
			return root, nil
		}
	}
	return "", errors.New("Neither world.dat nor world.sav could be found")
}

func extractSave(zipPath string, destDir string, token LockToken) error {
	fmt.Printf("Extracting save files from %s to %s...\n", zipPath, destDir)
	zroot, err := findSaveZipRoot(zipPath)
	if err != nil {
		return err
	}
	err = unzipFiles(zipPath, zroot, destDir, func(path string) bool {
		for _, m := range saveRegexes {
			if m.MatchString(path) {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	tokenFileName := "token.dftk"
	tokenPath := filepath.Join(destDir, tokenFileName)
	jstr, err := json.MarshalIndent(token, "", "\t")
	if err != nil {
		return err
	}
	ioutil.WriteFile(tokenPath, jstr, 0664)
	return nil
}
