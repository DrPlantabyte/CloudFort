package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"cloudfort/common"
	"cloudfort/server"
)

type ServerConfig struct {
	CloudFortVersion   string
	DFVersion          string
	WorldSaveFolder    string
	CheckOutTimeLimit  string
	DownloadTimeLimit  string
	WorldSizeLimitMB   float64
	TempFolder         string
	PortNumber         int64
	HostBindAddress    string // "0.0.0.0" for ipv4, "::" for ipv6
	ServerOverseerName string
}

var statusMap map[string]common.LockToken
var statusLock sync.Mutex

func main() {
	// first, initialize
	//thisFile, err := os.Executable()
	//fail(err)
	//thisDir := filepath.Dir(thisFile)
	config := initialize()

	// then start expiration watcher
	expTicker := time.NewTicker(time.Second)
	defer expTicker.Stop()
	done := make(chan bool)
	defer func() {
		done <- true
	}()
	go expirationChecker(expTicker, done, config)

	// finally, start network service
	hostStr := fmt.Sprintf("%s:%d", config.HostBindAddress, config.PortNumber)
	fmt.Printf("Starting server, listening to port %s...\n", hostStr)
	listener, err := net.Listen("tcp", hostStr)
	fail(err)

	defer listener.Close()

	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Printf("%v\n", err)
			continue
		}

		// If you want, you can increment a counter here and inject to handleClientRequest below as client identifier
		go handleClientRequest(connection, config)
	}
}

func handleClientRequest(conx net.Conn, config ServerConfig) {
	defer conx.Close()

	clientReader := bufio.NewReader(conx)

	// Waiting for the client message
	msg, err := clientReader.ReadString('\n')
	fmt.Printf("received %v", msg)

	if err == io.EOF {
		// client closed the connection
		log.Print("Client closed the connection")
		return
	} else if err != nil {
		log.Printf("%v\n", err)
		return
	}

	// Responding to the client message
	msg = strings.TrimSpace(msg)
	if msg == common.COM_CONCHECK {
		_, err = conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", common.RESP_CONCHECK)))
		warn(err)
	} else if strings.HasPrefix(msg, common.COM_STATUS) {
		fmt.Printf("Client %s requested status of all worlds\n", conx.RemoteAddr().String())
		// client requests list of worlds and their statuses
		jstr, err := json.Marshal(statusSnapshot(false))
		warn(err)
		_, err = conx.Write(jstr)
		warn(err)
	} else if strings.HasPrefix(msg, common.COM_RELEASE) {
		sp := strings.SplitN(msg, ":", 4)
		overseer := sp[1]
		worldName := sp[2]
		magicRunes := sp[3]
		fmt.Printf("Client %s requested that world %s revert to last check-in\n", conx.RemoteAddr().String(), worldName)
		tok, exists := getStatus(worldName)
		if !exists {
			e := errors.New(fmt.Sprintf("No world named '%s'", worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, e)))
			warn(e)
			return
		}
		if tok.MagicRunes == magicRunes {
			// valid overseer
			err = checkIn(worldName, overseer, config)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
		} else {
			e := errors.New(fmt.Sprintf("Overseer %s is not the currect holder of world %s", overseer, worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, e)))
			warn(e)
			return
		}
	} else if strings.HasPrefix(msg, common.COM_CHECKOUT) {
		sp := strings.Split(msg, ":")
		if len(sp) != 3 {
			e := errors.New(fmt.Sprintf("%s:%s '%s'\n", common.RESP_ERROR, "Invalid Check-out command", msg))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%v", e)))
			warn(e)
			return
		}
		overseer := sp[1]
		worldName := sp[2]
		fmt.Printf("Overseer %s from client %s requested to check-out world %s\n", overseer, conx.RemoteAddr().String(), worldName)
		wFilePath := filepath.Join(config.WorldSaveFolder, fmt.Sprintf("%s.zip", worldName))
		tok, exists := getStatus(worldName)
		jsdbg, _ := json.MarshalIndent(tok, "", " ")
		fmt.Printf("Current status token for %s:\n%s\n", worldName, string(jsdbg))
		if !exists {
			e := errors.New(fmt.Sprintf("No world named '%s'", worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, e)))
			warn(e)
			return
		} else if !common.FileExists(wFilePath) {
			e := errors.New(fmt.Sprintf("File '%s' not found", wFilePath))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, e)))
			warn(e)
			return
		} else if tok.Status != common.STATUS_AVAILABLE {
			// checked-out or otherwise unavailable
			e := errors.New(fmt.Sprintf("World named '%s' cannot be checked-out because it's unavailable (status == %s)", worldName, tok.Status))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, e)))
			warn(e)
			return
		} else {
			// Can check-out!
			fmt.Printf("Checking out world %s...\n", worldName)
			fmt.Printf("Hashing file...")
			hash, err := common.HashFile(wFilePath)
			fmt.Printf(" hash = '%s'\n", hash)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			fstat, err := os.Stat(wFilePath)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			fileSize := fstat.Size()
			// make sure the file is there
			fmt.Printf("Reading file %s\n", wFilePath)
			//buf := make([]byte, 0x100000)
			zipFileSrc, err := os.Open(wFilePath)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			// update the lock token
			fmt.Printf("Setting lock token to Downlaod\n")
			downloadLock := tok
			downloadLock.CurrentOverseer = overseer
			downloadLock.Status = common.STATUS_DOWNLOADING
			dd, err := time.ParseDuration(config.DownloadTimeLimit)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			cd, err := time.ParseDuration(config.CheckOutTimeLimit)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			downloadLock.Expires = time.Now().Add(dd).Format(time.RFC3339)
			downloadLock.MagicRunes = newMagicRunes()
			oldStatus, _ := setStatus(worldName, downloadLock, config)
			if oldStatus.Status != common.STATUS_AVAILABLE {
				// Oops! Thread race accident! Clean-up!
				setStatus(worldName, oldStatus, config)
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %s\n", common.RESP_ERROR, fmt.Sprintf("World named '%s' cannot be checked-out because it's unavailable (status == %s)", worldName, oldStatus.Status))))
				return
			}
			// prepare the checkout token (a copy will be sent to the client)
			checkoutToken := downloadLock
			checkoutToken.Status = common.STATUS_CHECKOUT
			checkoutToken.Expires = time.Now().Add(cd).Format(time.RFC3339)
			tjstr, err := json.Marshal(checkoutToken)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			}
			// finally, do the file transfer
			fmt.Printf("Transmitting download response...\n")
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", common.RESP_DOWNLOAD)))
			fmt.Printf("Transmitting token '%s'...\n", tjstr)
			conx.Write(tjstr)
			conx.Write(common.StrToUtf8("\n"))
			fmt.Printf("Transmitting file hash...\n")
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", hash)))
			fmt.Printf("Transmitting file data...\n")
			//io.CopyBuffer(conx, zipFileSrc, buf)
			err = common.SendFile(zipFileSrc, conx, fileSize, false)
			if err != nil {
				warn(err)
				err2 := checkIn(worldName, overseer, config)
				if err2 != nil {
					warn(err2)
				}
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				return
			}
			_, err = setStatus(worldName, checkoutToken, config)
			if err != nil {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
				warn(err)
				return
			} else {
				conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", common.RESP_SUCCESS)))
			}
			writeHistoryLine(time.Now(), worldName, overseer, fmt.Sprintf("World pulled from the cosmic aether by overseer %s", overseer), config)
			fmt.Printf("...checkout done\n")
		}
		return
	} else if strings.HasPrefix(msg, common.COM_CHECKIN) {
		sp := strings.SplitN(msg, ":", 4)
		overseer := sp[1]
		worldName := sp[2]
		magicRunes := sp[3]
		fmt.Printf("Overseer %s from client %s requested to check-in world %s\n", overseer, conx.RemoteAddr().String(), worldName)
		// first, check if client has permission to check-in this world
		lok, exists := getStatus(worldName)
		if !exists {
			err = errors.New(fmt.Sprintf("World %s does not exist on server", worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		if lok.Status == common.STATUS_AVAILABLE {
			err = errors.New(fmt.Sprintf("World %s cannot be checked in because it has already been checked in", worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		if lok.MagicRunes != magicRunes {
			err = errors.New(fmt.Sprintf("Overseer %s is not the currect holder of world %s", overseer, worldName))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		// next, tell client that they may check-in
		fmt.Printf("Permission granted for check-in\n")
		conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", common.RESP_UPLOAD)))
		// next, read the upload file hash
		fmt.Printf("Reading upload file hash...\n")
		msg, err = clientReader.ReadString('\n')
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		hash := strings.TrimSpace(msg)
		fmt.Printf("%s\n", hash)
		// now read the file from the client
		tmpFilePath := filepath.Join(config.TempFolder, fmt.Sprintf("CloudFort-upload-%s.temp", worldName))
		fmt.Printf("Receiving file data to temp file %s\n", tmpFilePath)
		defer os.Remove(tmpFilePath)
		tmpFile, err := os.OpenFile(tmpFilePath, os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		//buf := make([]byte, 0x100000)
		//_, err = io.CopyBuffer(tmpFile, clientReader, buf)
		err = common.RecvFile(clientReader, tmpFile, false)
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		err = tmpFile.Close()
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		// check the hash to make sure the file is good
		tmpHash, err := common.HashFile(tmpFilePath)
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		fmt.Printf("Hash check:\n%s <- transmitted hash\n%s <- actual hash\n", hash, tmpHash)
		if hash != tmpHash {
			err = errors.New(fmt.Sprintf("File hash mis-match"))
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		// data is good!
		// now replace save zip with files from new one
		wFilePath := filepath.Join(config.WorldSaveFolder, fmt.Sprintf("%s.zip", worldName))
		backupPath := fmt.Sprintf("%s.backup", wFilePath)
		err = os.Rename(wFilePath, backupPath) // backup existing save incase we need to undo
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		err = copySave(tmpFilePath, wFilePath, config)
		if err != nil {
			os.Rename(backupPath, wFilePath)
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		// finally mark the world as checked-in
		err = checkIn(worldName, overseer, config)
		if err != nil {
			conx.Write(common.StrToUtf8(fmt.Sprintf("%s: %v\n", common.RESP_ERROR, err)))
			warn(err)
			return
		}
		// success!
		fmt.Printf("Check-in sucessful!\n")
		conx.Write(common.StrToUtf8(fmt.Sprintf("%s\n", common.RESP_SUCCESS)))

	} else {
		// command not recognized
		warn(errors.New(fmt.Sprintf("Command %s not recognized", msg)))
	}
	// done
}

func expirationChecker(ticker *time.Ticker, done chan bool, config ServerConfig) {
	// periodically check expiration times
	for {
		select {
		case <-done:
			fmt.Println("Experiation timer ticker terminated.")
			return
		case _ = <-ticker.C:
			tnow := time.Now()
			smap := statusSnapshot(true)
			for world, token := range smap {
				if token.Status != common.STATUS_AVAILABLE {
					expTime, err := time.Parse(time.RFC3339, token.Expires)
					if err != nil {
						warn(errors.Wrap(err, "Error parsing exipration date"))
					}
					if err != nil || tnow.After(expTime) {
						fmt.Printf("Lock for world %s has expired. Resetting status to %s\n", world, common.STATUS_AVAILABLE)
						err = checkIn(world, config.ServerOverseerName, config)
						if err != nil {
							warn(errors.Wrapf(err, "Error checking-in world %s", world))
						}
					}
				}
			}
		}
	}
}

func initialize() ServerConfig {
	// init global variables
	statusMap = make(map[string]common.LockToken)
	fmt.Print("Loading configuration...")
	// first, load the config settings (saving the default if there is no config file)
	defaultConfig := ServerConfig{
		CloudFortVersion:   common.CloudFortVersion,
		DFVersion:          "0.47.05",
		WorldSaveFolder:    "save",
		CheckOutTimeLimit:  "8h",
		DownloadTimeLimit:  "30m",
		WorldSizeLimitMB:   256,
		TempFolder:         "temp",
		PortNumber:         13137,
		HostBindAddress:    "0.0.0.0",
		ServerOverseerName: "<Server>",
	}
	var config ServerConfig
	configFile := "server-config.json"
	if !common.FileExists(configFile) {
		fmt.Println("Config file does not exist. Creating a new one...")
		jstr, _ := json.MarshalIndent(defaultConfig, "", "\t")
		err := ioutil.WriteFile(configFile, jstr, 0664)
		fail(err)
		config = defaultConfig
	} else {
		jstr, err := ioutil.ReadFile(configFile)
		fail(err)
		config = defaultConfig
		err = json.Unmarshal(jstr, &config)
		fail(err)
	}
	err := serverSanityCheck(config)
	fail(err)
	fmt.Println("Done.")
	// then make the directories
	fmt.Print("Initializing folders...")
	saveDir := config.WorldSaveFolder
	newDir, err := ensureDir(saveDir)
	fail(err)
	if newDir {
		// if there wasn't a save directory before, seed it with a demo world as an example
		data, fname := server.GetDemoWorld()
		err := ioutil.WriteFile(filepath.Join(saveDir, fname), data, 0664)
		fail(err)
	}
	_, err = ensureDir(config.TempFolder)
	fail(err)
	historyFile := filepath.Join(config.WorldSaveFolder, "history.csv")
	if !common.FileExists(historyFile) {
		err := ioutil.WriteFile(historyFile, []byte("Time,World,Overseer,Event\n"), 0664)
		fail(err)
	}
	fmt.Println("Done.")
	// then mark any un-tracked (ie new) .zip files as checked-in
	zipFiles, err := common.ListFiles(saveDir, ".zip")
	fail(err)
	for _, f := range zipFiles {
		worldName := common.NameFromFile(f)
		lockFile := common.SwapFileSuffix(f, ".dftk")
		if !common.FileExists(lockFile) {
			err = checkIn(worldName, config.ServerOverseerName, config)
			warn(err)
		}
	}
	// now read all .dftk files to sycronize world status
	lockFiles, err := common.ListFiles(saveDir, ".dftk")
	for _, f := range lockFiles {
		zipFile := common.SwapFileSuffix(f, ".zip")
		if !common.FileExists(zipFile) {
			warn(errors.New(fmt.Sprintf("%s found, but %s does not exist. Skipping.", f, zipFile)))
			continue
		}
		jstr, err := ioutil.ReadFile(f)
		fail(err)
		var token common.LockToken
		err = json.Unmarshal(jstr, &token)
		fail(err)
		worldName := common.NameFromFile(f)
		statusMap[worldName] = token
	}
	// finally, return the config
	return config
}

func serverSanityCheck(c ServerConfig) error {
	_, err := time.ParseDuration(c.CheckOutTimeLimit)
	if err != nil {
		return err
	}
	_, err = time.ParseDuration(c.DownloadTimeLimit)
	if err != nil {
		return err
	}
	return nil
}

func getStatus(worldName string) (common.LockToken, bool) {
	// first, lock to prevent de-sync
	statusLock.Lock()
	defer statusLock.Unlock()
	// then look-up the map
	token, present := statusMap[worldName]
	return token, present
}

func setStatus(worldName string, newStatus common.LockToken, config ServerConfig) (common.LockToken, error) {
	// first, lock to prevent de-sync
	statusLock.Lock()
	defer statusLock.Unlock()
	// update map
	oldToken := statusMap[worldName]
	statusMap[worldName] = newStatus
	// save new state to file
	lockFile := filepath.Join(config.WorldSaveFolder, fmt.Sprintf("%s.dftk", worldName))
	jstr, _ := json.MarshalIndent(newStatus, "", "\t")
	fmt.Printf("Setting status of %s to \n%s\n", worldName, jstr)
	err := ioutil.WriteFile(lockFile, jstr, 0664)
	if err != nil {
		return oldToken, err
	}
	return oldToken, nil
}

func statusSnapshot(showMagicRunes bool) map[string]common.LockToken {
	// first, lock to prevent de-sync
	statusLock.Lock()
	defer statusLock.Unlock()
	// then do work
	var cp map[string]common.LockToken
	cp = make(map[string]common.LockToken)
	for k, v := range statusMap {
		v2 := v
		if !showMagicRunes {
			v2.MagicRunes = ""
		}
		cp[k] = v2
	}
	return cp
}

func checkIn(worldName string, overseer string, config ServerConfig) error {
	tnow := time.Now()
	token := common.LockToken{
		Status:          common.STATUS_AVAILABLE,
		Expires:         tnow.Format(time.RFC3339),
		CurrentOverseer: overseer,
		MagicRunes:      "0",
	}
	_, err := setStatus(worldName, token, config)
	if err != nil {
		_ = writeHistoryLine(tnow, worldName, overseer, fmt.Sprintf("World lost in space and time: %v", err), config)
		return err
	}
	err = writeHistoryLine(tnow, worldName, overseer, "World returned to the cosmic aether", config)
	return err
}

func newMagicRunes() string {
	n := 10
	bb := make([]byte, n, n)
	_, err := rand.Read(bb)
	if err != nil {
		// crypto random not working, fall-back mode
		bb = []byte(fmt.Sprintf("%d", time.Now().Nanosecond()))
	}
	return base64.StdEncoding.EncodeToString(bb)
}

func writeHistoryLine(t time.Time, world string, overseer string, event string, config ServerConfig) error {
	histFile := filepath.Join(config.WorldSaveFolder, "history.csv")
	file, err := os.OpenFile(histFile, os.O_APPEND|os.O_WRONLY, 0664)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(fmt.Sprintf("%s,\"%s\",\"%s\",\"%s\"\n", t.Format(time.RFC3339), world, overseer, event))
	if err != nil {
		return err
	}
	return nil
}

func copySave(srcZip string, destZip string, config ServerConfig) error {
	fmt.Printf("Extracting save files from %s to %s...\n", srcZip, destZip)
	var token common.LockToken
	tmpDir := filepath.Join(config.TempFolder, common.NameFromFile(destZip))
	defer common.DeleteDir(tmpDir)
	err := os.MkdirAll(tmpDir, 0775)
	if err != nil {
		return err
	}
	err = common.ExtractSave(srcZip, tmpDir, token)
	if err != nil {
		return err
	}
	files, err := common.ScanDir(tmpDir)
	if err != nil {
		return err
	}
	// filter-out the lock token
	files = common.FilterStrings(files, func(g string) bool {
		return !strings.HasSuffix(g, ".dftk")
	})
	err = common.ZipFiles(tmpDir, files, destZip)
	if err != nil {
		return err
	}
	return nil
}

// returns true if it made a new dir
func ensureDir(dirPath string) (bool, error) {
	if !common.FileExists(dirPath) {
		return true, os.MkdirAll(dirPath, 0777)
	}
	return false, nil
}

func fail(e error) {
	if e != nil {
		log.Print(e)
		fmt.Printf("ERROR: %v\n", e)
		fmt.Printf("Program terminated due to error.\n")
		os.Exit(1)
	}
}
func warn(e error) {
	if e != nil {
		log.Print(e)
		fmt.Printf("WARNING: %v\n", e)
	}
}
