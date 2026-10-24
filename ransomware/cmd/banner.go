package cmd

import (
	"os"

	"github.com/fatih/color"
)

var (
	banner = `
            __  __                     ___      __     
            \ \/ /___ _   _   ______ _/ (_)____/ /____ 
             \  / __  /  | | / / __  / / / ___/ __/ _ \
             / / /_/ /   | |/ / /_/ / / (__  ) /_/  __/
            /_/\__,_/    |___/\__,_/_/_/____/\__/\___/
    `
	delimiter = `
    ===============================================================================================
    `
)

// Print banner
func PrintBanner() {
	color.New(color.FgCyan).Add(color.Bold).Println(delimiter)
	color.New(color.FgRed).Add(color.Bold).Println(banner)
	color.New(color.FgCyan).Add(color.Bold).Println(delimiter)
}

// Print script usage
func Usage(errmsg string) {
	color.New(color.FgWhite).Add(color.Bold).Printf(
		"%s\n\n"+
			"usage: %s\n"+
			"       %s decrypt key\n"+
			"      Will try to decrypt your files using the given key\n",
		errmsg, os.Args[0], os.Args[0])
	os.Exit(2)
}
