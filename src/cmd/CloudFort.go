package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/gen2brain/dlgs"
	"github.com/sqweek/dialog"

	"cloudfort/common"
)

type ClientConfig struct {
	OverseerName     string
	CloudFortVersion string
	HostName         string
	PortNumber       int64
}

func main() {
	fmt.Println("Starting ClodFort client...")
	fmt.Println("DO NOT CLOSE THIS WINDOW!!!")
	fmt.Println("Closing this window will NOT corrupt any data nor cause any harm, but closing this window will forcibly terminate the program, interrupting any file transfers.")
	// first, initialize and setup the environment
	thisFile, err := os.Executable()
	errCheck(err)
	thisDir := filepath.Dir(thisFile)
	err = os.Chdir(thisDir)
	errCheck(err)
	var dfPath string // "df" or "dfhack" on linux and Mac, "Dwarf Fortress.exe" on windows
	foundDF := false
	for _, binName := range []string{"Dwarf Fortress.exe", "dfhack", "df"} {
		bp := filepath.Join(thisDir, binName)
		if common.FileExists(bp) {
			dfPath = bp
			foundDF = true
			break
		}
	}
	if !foundDF {
		errCheck(errors.New("Dwarf Fortress executable not found!"))
		os.Exit(1)
	}
	saveDir := filepath.Join(thisDir, "data", "save")
	os.MkdirAll(saveDir, 0777) // safely does nothing if directory already exists

	fmt.Printf("Starting CloudFort in %s...\n", thisDir)

	defaultConfig := ClientConfig{
		CloudFortVersion: common.CloudFortVersion,
		HostName:         "localhost",
		PortNumber:       13137,
		OverseerName:     "",
	}
	var config ClientConfig
	configFile := filepath.Join(thisDir, "CloudFort-config.json")
	if !common.FileExists(configFile) {
		fmt.Println("Config file does not exist. Creating a new one...")
		fmt.Println("...asking overseer for their name...")
		for {
			name, ok, err := dlgs.Entry("Identify yourself!", "What is your name, overseer?", "")
			errCheck(err)
			name = strings.TrimSpace(name)
			if ok && validateName(name) {
				defaultConfig.OverseerName = name
				break
			} else {
				errorPopup("That name is not acceptable! Try again.")
			}
		}
		fmt.Println("...asking overseer for server address...")
		for {
			hostStr, ok, err := dlgs.Entry("Cloud Address", "Enter the server address and port number, separated by a : colon. For example, localhost:13137 or 192.168.0.107:13137 or mycloud.net:8013", fmt.Sprintf("%s:%d", defaultConfig.HostName, defaultConfig.PortNumber))
			errCheck(err)
			sa := strings.SplitN(hostStr, ":", 2)
			defaultConfig.HostName = sa[0]
			defaultConfig.PortNumber, err = strconv.ParseInt(sa[1], 10, 0)
			if err == nil && ok {
				resp, err := textServer(defaultConfig.HostName, int(defaultConfig.PortNumber), common.COM_CONCHECK)
				if err == nil && strings.TrimSpace(resp) == common.RESP_CONCHECK {
					// connection good
					break
				} else {
					errorPopup(fmt.Sprintf("Unable to contact server. \n\n%v", err))
					if !askUser("Edit server address and try again?", "Try again?") {
						os.Exit(0)
					}
				}
			} else {
				errorPopup("That address is not acceptable! Try again.")
			}
		}
		fmt.Println("...creating config file...")
		jstr, _ := json.MarshalIndent(defaultConfig, "", "\t")
		err := ioutil.WriteFile(configFile, jstr, 0664)
		errCheck(err)
		config = defaultConfig
		infoPopup("Success!", fmt.Sprintf("Success! You can change your overseer name and server address by editing the file %s", configFile))
	} else {
		jstr, err := ioutil.ReadFile(configFile)
		errCheck(err)
		config = defaultConfig
		err = json.Unmarshal(jstr, &config)
		errCheck(err)
		infoPopup(fmt.Sprintf("Welcome, %s!", config.OverseerName), fmt.Sprintf(
			"Welcome, %s, to CloudFort! You can check-out worlds from CloudFort server %s:%d. Note that you can change your overseer name and server address by editing the file %s",
			config.OverseerName, config.HostName, config.PortNumber, configFile))
	}
	err = sanityCheck(config)
	errCheck(err)

	fmt.Printf("...checking save folders for left-over check-outs...\n")
	keepCheckedOutWorld, err := checkWorldDirs(saveDir, config)
	errCheck(err)

	fmt.Printf("...connecting to server...\n")
	hostName := config.HostName
	portNum := int(config.PortNumber)
	fmt.Printf("\tHost: %s\n\tPort: %d\n", hostName, portNum)
	if !keepCheckedOutWorld {
		msg, err := textServer(hostName, portNum, common.COM_STATUS)
		errCheck(err)
		fmt.Println(msg)
		var worlds map[string]common.LockToken
		err = json.Unmarshal([]byte(msg), &worlds)
		errCheck(err)
		worldLabels := make([]string, 0, 32)
		label2WorldMap := make(map[string]string)
		for k, v := range worlds {
			fmt.Printf("%s: %s\n", k, v.Status)
			wl := fmt.Sprintf("%s: %s", k, v.Status)
			worldLabels = append(worldLabels, wl)
			label2WorldMap[wl] = k
		}
		item, _, err := dlgs.List("CloudFort World Selection", "Select a world:", worldLabels)
		errCheck(err)
		if item == "" {
			fmt.Println("No world selected.")
		} else {
			worldSelect := label2WorldMap[item]
			fmt.Printf("Selected %s", worldSelect)
			err := checkOut(worldSelect, saveDir, config)
			errCheck(err)
		}
	}

	//os.Exit(0)

	fmt.Println("Starting Dwarf Fortress...")

	dfCmd := exec.Command(dfPath)
	_ = dfCmd.Run() // blocks until subprocess terminates
	//errCheck(errors.Wrapf(err, "Dwarf Fortress executable '%s' failed to run or terminated with error status", dfPath))
	// DF returns error code even on normal exit

	fmt.Println("...DF closed. Checking-in CloudFort worlds,please do not close this window...")
	_, err = checkWorldDirs(saveDir, config)
	errCheck(err)

	fmt.Println("...Complete! Terminating CloudFort...")

	fmt.Println("...Done!")

	os.Exit(0)

}

func checkWorldDirs(saveDir string, config ClientConfig) (bool, error) {
	keepCheckoutWorld := false
	saveWorldDirs, err := common.ListDirs(saveDir)
	if err != nil {
		return keepCheckoutWorld, err
	}
	for _, worldDirPath := range saveWorldDirs {
		tokenPath := filepath.Join(worldDirPath, "token.dftk")
		if common.FileExists(tokenPath) {
			jstr, err := ioutil.ReadFile(tokenPath)
			var checkoutToken common.LockToken
			err = json.Unmarshal(jstr, &checkoutToken)
			errCheck(err)
			worldName := filepath.Base(worldDirPath)
			yesCheckIn := askUser(
				fmt.Sprintf("World '%s' has been checked-out from the server. Would you like to check-in this world?", worldName), "Check-in world?")
			yesRevert := false
			if !yesCheckIn {
				yesRevert = askUser(
					fmt.Sprintf("Do you want to revert world '%s' to it's previous state, losing any changes to since check-out?", worldName), "Revert world?")
			}
			if yesCheckIn {
				fmt.Printf("User requested to check-in %s\n", worldName)
				err := checkIn(worldDirPath, checkoutToken, config)
				errCheck(err)
			} else if yesRevert {
				fmt.Printf("User requested to revert %s\n", worldName)
				err := cancelCheckOut(worldName, saveDir, config.OverseerName, checkoutToken.MagicRunes, config)
				errCheck(err)
			} else {
				keepCheckoutWorld = true
			}
		}
	}
	return keepCheckoutWorld, nil
}

func checkIn(worldDir string, token common.LockToken, config ClientConfig) error {
	world := filepath.Base(worldDir)
	fmt.Printf("Checking in world %s\n", world)
	// first, zip the save to a temp file
	tmpFile, err := os.CreateTemp("", "CloudFort-upload.*.temp")
	if err != nil {
		return err
	}
	err = tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	zipPath := tmpFile.Name()
	if err != nil {
		return err
	}
	fmt.Printf("Zipping region folder to %s...\n", zipPath)
	saveFiles, err := common.ScanDir(worldDir)
	if err != nil {
		return err
	}
	saveFiles = common.FilterStrings(saveFiles, saveFileFilter)
	err = common.ZipFiles(worldDir, saveFiles, zipPath)
	if err != nil {
		return err
	}
	fmt.Printf("Hashing file %s...\n", zipPath)
	hash, err := common.HashFile(zipPath)
	if err != nil {
		return err
	}
	fstat, err := os.Stat(zipPath)
	if err != nil {
		return err
	}
	// next, connect to the server
	hostStr := fmt.Sprintf("%s:%d", config.HostName, config.PortNumber)
	fmt.Printf("Contacting server %s\n", hostStr)
	connection, err := net.Dial("tcp", hostStr)
	if err != nil {
		return errors.New(fmt.Sprintf("errChecked to connect to server %s \n\t%v", hostStr, err))
	}
	defer connection.Close()
	// send check-in request
	fmt.Printf("Requesting checkin\n")
	serverReader := bufio.NewReader(connection)
	_, err = connection.Write(common.StrToUtf8(fmt.Sprintf("%s:%s:%s:%s\n", common.COM_CHECKIN, config.OverseerName, world, token.MagicRunes)))
	if err != nil {
		return err
	}
	resp, err := serverReader.ReadString('\n')
	fmt.Printf("Received %s\n", resp)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resp) == common.RESP_UPLOAD {
		// server gave the go-ahead, proceed
		fmt.Printf("Sending hash %s\n", hash)
		_, err = connection.Write(common.StrToUtf8(fmt.Sprintf("%s\n", hash)))
		if err != nil {
			return err
		}
		// now transmit the file
		fmt.Printf("Sending file data\n")
		tf, err := os.Open(zipPath)
		if err != nil {
			return err
		}
		//buf := make([]byte, 0x100000)
		//_, err = io.CopyBuffer(connection, tf, buf)
		err = common.SendFile(tf, connection, fstat.Size(), true)
		if err != nil {
			return err
		}
		// did it succeed?
		resp, err := serverReader.ReadString('\n')
		fmt.Printf("Server response: %s\n", resp)
		if strings.TrimSpace(resp) != common.RESP_SUCCESS {
			err = errors.New(resp)
			return err
		}
		err = common.DeleteDir(worldDir)
		if err != nil {
			return err
		}
		fmt.Printf("Check-in complete\n")
	} else {
		e := errors.New(resp)
		return e
	}
	return nil
}

func checkOut(world string, saveDir string, config ClientConfig) error {
	// check for name collision
	dirPath := filepath.Join(saveDir, world)
	if common.FileExists(dirPath) {
		// save folder already exists!
		return errors.New(fmt.Sprintf("Cannot checkout save for world %s because save folder %s already exists", world, dirPath))
	}
	// first, request checkout from server and see if it is available
	hostStr := fmt.Sprintf("%s:%d", config.HostName, config.PortNumber)
	fmt.Printf("Contacting server %s\n", hostStr)
	connection, err := net.Dial("tcp", hostStr)
	if err != nil {
		return errors.New(fmt.Sprintf("errChecked to connect to server %s \n\t%v", hostStr, err))
	}
	defer connection.Close()
	//
	fmt.Printf("Requesting checkout\n")
	serverReader := bufio.NewReader(connection)
	_, err = connection.Write(common.StrToUtf8(fmt.Sprintf("%s:%s:%s\n", common.COM_CHECKOUT, config.OverseerName, world)))
	if err != nil {
		return err
	}
	resp, err := serverReader.ReadString('\n')
	fmt.Printf("Received %s\n", resp)
	if err != nil {
		return err
	}
	if strings.HasPrefix(resp, common.RESP_ERROR) {
		e := errors.New(resp)
		return e
	} else if strings.TrimSpace(resp) == common.RESP_DOWNLOAD {
		// yes it is available, proceed to download
		// first, read the lock token for the magic rune sequence
		jstr, err := serverReader.ReadString('\n')
		fmt.Printf("Received %s\n", jstr)
		if err != nil {
			return err
		}
		var checkoutToken common.LockToken
		err = json.Unmarshal([]byte(jstr), &checkoutToken)
		if err != nil {
			return err
		}
		dbgjstr, _ := json.MarshalIndent(checkoutToken, "", " ")
		fmt.Printf("checkout token:\n%s\n", string(dbgjstr))
		// read the file hash
		hash_, err := serverReader.ReadString('\n')
		hash := strings.TrimSpace(hash_)
		// then download zip file from server to a temp file
		//tmpDir := os.TempDir()
		outFile, err := os.CreateTemp("", "CloudFort-download.*.temp")
		if err != nil {
			return err
		}
		fmt.Printf("Downloading to temp file %s\n", outFile.Name())
		defer os.Remove(outFile.Name())
		//buf := make([]byte, 0x100000)
		//_, err = io.CopyBuffer(outFile, serverReader, buf)
		err = common.RecvFile(serverReader, outFile, true, -1)
		if err != nil {
			return err
		}
		err = outFile.Close()
		if err != nil {
			return err
		}
		fmt.Printf("Data transferred!\n")
		undoFunc := func(e error) {
			fmt.Printf("Check-out failed, checking back in...\n")
			err2 := cancelCheckOut(world, saveDir, config.OverseerName, checkoutToken.MagicRunes, config)
			if err2 != nil {
				errCheck(errors.New(fmt.Sprintf("Double error: %v; %v", e, err2)))
			}
		}
		// now check the hashes to guard against incomplete (or tampered) data transfer
		fhash, err := common.HashFile(outFile.Name())
		errCheck(err)
		fmt.Printf("Hash check:\n    server hash: %s\n  download hash: %s\n", hash, fhash)
		if hash != fhash {
			fmt.Println("FAILURE: file hash mismatch!")
			// uh-oh, files don't match
			defer undoFunc(err)
		}
		// finally, extract only relevant files from download to save folder
		fmt.Printf("Extracting files from %s to %s\n", outFile.Name(), dirPath)
		err = common.ExtractSave(outFile.Name(), dirPath, checkoutToken)
		if err != nil {
			defer undoFunc(err)
			return err
		}
		fmt.Printf("...Done!\n")
	}
	return nil
}

func cancelCheckOut(world string, saveDir string, overseer string, worldMagicRunes string, config ClientConfig) error {
	// tell server to make this world available again without checking it back in
	resp, err := textServer(config.HostName, int(config.PortNumber), fmt.Sprintf("%s:%s:%s:%s", common.COM_RELEASE, overseer, world, worldMagicRunes))
	if err != nil {
		return err
	}
	if strings.HasPrefix(resp, common.RESP_ERROR) {
		e := errors.New(resp)
		return e
	}
	worldDir := filepath.Join(saveDir, world)
	if common.FileExists(worldDir) {
		err = common.DeleteDir(worldDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func sanityCheck(config ClientConfig) error {
	if strings.ContainsRune(config.OverseerName, ':') {
		return errors.New("Invalid overseer name: name must not contain ':'")
	}
	return nil
}

func errCheck(e error) {
	if e != nil {
		log.Print(e)
		errorPopup(fmt.Sprintf("%v", e))
		os.Exit(1)
	}
}
func errorPopup(msg string) {
	dialog.Message("%s", msg).Title("Error!").Error()
}
func infoPopup(title, msg string) {
	dialog.Message("%s", msg).Title(title).Info()
}
func askUser(question string, title string) bool {
	yes := dialog.Message("%s", question).Title(title).YesNo()
	return yes
}

func textServer(hostName string, portNum int, msg string) (string, error) {
	hostStr := fmt.Sprintf("%s:%d", hostName, portNum)
	connection, err := net.Dial("tcp", hostStr)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Failed to connect to server %s \n\t%v", hostStr, err))
	}
	defer connection.Close()
	//
	serverReader := bufio.NewReader(connection)
	_, err = connection.Write(common.StrToUtf8(fmt.Sprintf("%s\n", msg)))
	fmt.Printf("sent %v\n", msg)
	if err != nil {
		errMsg := fmt.Sprintf("I/O error: errChecked to send message to server \n\t%v", err)
		return "", errors.New(errMsg)
	}
	serverResponse, err := serverReader.ReadString('\n')
	fmt.Printf("received %v\n", serverResponse)
	return serverResponse, nil
}
func saveFileFilter(s string) bool {
	for _, regex := range common.SaveRegexes {
		if regex.MatchString(s) {
			return true
		}
	}
	return false
}
func validateName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, r := range []rune{':', ';', '/', '\\', '\n', '\t', '%'} {
		if strings.ContainsRune(name, r) {
			return false
		}
	}
	return true
}
