// Ventana persistente "Abrazo" para Windows, con música de fondo en bucle.
// Compilar con:  go build -ldflags "-H=windowsgui" -o abrazo.exe main.go
//
// IMPORTANTE: coloca un archivo WAV llamado "Theme.wav" en esta misma carpeta
// antes de compilar (se incrusta en el ejecutable vía go:embed).
//
// Cierre ventana + música:  Ctrl + Shift + Ñ + 3  (simultáneas)
// Silenciar/reactivar solo la música:  Ctrl + Shift + M
//
// Modo de depuración para averiguar el VK code real de tu tecla Ñ:
//   go build -o abrazo_debug.exe main.go   (sin -H=windowsgui, para ver consola)
//   abrazo_debug.exe -debug

package main

import (
	_ "embed"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

//go:embed Theme.wav
var audioData []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")

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
	procGetDC             = user32.NewProc("GetDC")
	procReleaseDC         = user32.NewProc("ReleaseDC")
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procDrawText          = user32.NewProc("DrawTextW")
	procGetClientRect     = user32.NewProc("GetClientRect")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")

	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")

	procCreateFont   = gdi32.NewProc("CreateFontW")
	procSelectObject = gdi32.NewProc("SelectObject")
	procDeleteObject = gdi32.NewProc("DeleteObject")
	procSetBkMode    = gdi32.NewProc("SetBkMode")
	procSetTextColor = gdi32.NewProc("SetTextColor")

	procRegOpenKeyEx   = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	procRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	procRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
	WS_CAPTION     = 0x00C00000
	WS_VISIBLE     = 0x10000000
	WS_POPUP       = 0x80000000
	WS_EX_TOPMOST  = 0x00000008
	SW_SHOW        = 5
	CW_USEDEFAULT  = -2147483648
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
	// VK de la tecla Ñ en teclado español: posición física de VK_OEM_1 (0xBA).
	// Si en tu equipo no coincide, usa el modo -debug para verificar el código real.
	VK_ENYE = 0xBA

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
	allowClose = false
	hookHandle uintptr
	muted      = false

	// Latches para que, al mantener presionadas las teclas, el atajo
	// se dispare una sola vez (no en cada repetición de WM_KEYDOWN).
	closeComboLatched = false
	muteComboLatched  = false
)

// playLoop reproduce el audio incrustado en bucle, en segundo plano.
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

// stopAudio detiene cualquier reproducción en curso.
func stopAudio() {
	procPlaySound.Call(0, 0, uintptr(SND_PURGE))
}

// toggleMute silencia o reactiva la música sin cerrar la ventana.
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
	title, _ := syscall.UTF16PtrFromString("Abrazo")

	hInstance, _, _ := procGetModuleHandle.Call(0)

	var wc WNDCLASSEX
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = syscall.NewCallback(wndProc)
	wc.hInstance = syscall.Handle(hInstance)
	wc.lpszClassName = className
	cursor, _, _ := procLoadCursor.Call(0, 32512) // IDC_ARROW
	wc.hCursor = syscall.Handle(cursor)
	wc.hbrBackground = syscall.Handle(16) // COLOR_WINDOW + 1 aprox (blanco)

	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	screenW, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN
	screenH, _, _ := procGetSystemMetrics.Call(1) // SM_CYSCREEN

	width, height := 520, 220
	x := (int(screenW) - width) / 2
	y := (int(screenH) - height) / 2

	hwnd, _, _ := procCreateWindowEx.Call(
		uintptr(WS_EX_TOPMOST),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(WS_CAPTION|WS_VISIBLE), // sin WS_SYSMENU: no hay botón de cerrar
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)

	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	playLoop()

	// Hook de teclado global para detectar la combinación de cierre
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
		return 0 // si no, se ignora: la ventana no se cierra
	case WM_SYSCOMMAND:
		// Bloquea SC_CLOSE (0xF060) proveniente de Alt+F4 u otros caminos
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

	procSetBkMode.Call(hdc, TRANSPARENT)
	procSetTextColor.Call(hdc, 0x00202020)

	// Fuente para el título
	titleFont, _, _ := procCreateFont.Call(
		uintptr(28), 0, 0, 0, uintptr(700), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Segoe UI"))),
	)
	oldFont, _, _ := procSelectObject.Call(hdc, titleFont)

	titleRect := RECT{rect.Left + 10, rect.Top + 20, rect.Right - 10, rect.Top + 90}
	titleText, _ := syscall.UTF16PtrFromString("Has sido abrazado por Alecs y juano")
	procDrawText.Call(
		hdc, uintptr(unsafe.Pointer(titleText)), ^uintptr(0),
		uintptr(unsafe.Pointer(&titleRect)),
		uintptr(DT_CENTER|DT_VCENTER|DT_SINGLELINE),
	)

	procSelectObject.Call(hdc, oldFont)
	procDeleteObject.Call(titleFont)

	// Fuente para el subtítulo
	subFont, _, _ := procCreateFont.Call(
		uintptr(16), 0, 0, 0, uintptr(400), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Segoe UI"))),
	)
	oldFont2, _, _ := procSelectObject.Call(hdc, subFont)

	subRect := RECT{rect.Left + 10, rect.Top + 100, rect.Right - 10, rect.Bottom - 10}
	subText, _ := syscall.UTF16PtrFromString("Un abrazo virtual, pero con todo el cariño real.")
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

// keyboardHookProc detecta las combinaciones de cierre (Ctrl+Shift+Ñ+3)
// y de silenciar música (Ctrl+Shift+M), cada una disparándose una sola vez
// por pulsación gracias a los latches.
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

// registerAutostart añade el ejecutable actual a HKCU\...\Run para que
// arranque automáticamente con Windows.
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

// runDebugMode imprime en consola el VK code de cada tecla pulsada,
// útil para confirmar el código real de la tecla Ñ en tu teclado.
func runDebugMode() {
	fmt.Println("Modo debug: pulsa teclas para ver su VK code (Ctrl+C para salir)")
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