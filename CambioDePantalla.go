package main

import (
	_ "embed"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

//go:embed ransomware/images.jpg
var imageData []byte

const (
	spiSetDeskWallpaper = 0x0014
	spifUpdateIniFile   = 0x01
	spifSendChange      = 0x02
)

func main() {
	// Write embedded image to a temporary file
	tmpFile, err := os.CreateTemp("", "wallpaper_*.jpg")
	if err != nil {
		fmt.Println("Error creando archivo temporal:", err)
		os.Exit(1)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(imageData); err != nil {
		fmt.Println("Error escribiendo imagen:", err)
		os.Exit(1)
	}
	tmpFile.Close()

	pathPtr, err := syscall.UTF16PtrFromString(tmpFile.Name())
	if err != nil {
		fmt.Println("Error al convertir la ruta:", err)
		os.Exit(1)
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")

	ret, _, callErr := systemParametersInfo.Call(
		uintptr(spiSetDeskWallpaper),
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(spifUpdateIniFile|spifSendChange),
	)
	if ret == 0 {
		fmt.Println("No se pudo cambiar el fondo:", callErr)
		os.Exit(1)
	}

	fmt.Println("Fondo de pantalla cambiado correctamente")
}