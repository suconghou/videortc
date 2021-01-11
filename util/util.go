package util

import (
	"log"
	"os"
)

var (
	// Log print to stdout
	Log = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
)
