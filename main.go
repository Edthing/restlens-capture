package main

import (
	"os"

	"github.com/Edthing/restlens-capture/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
