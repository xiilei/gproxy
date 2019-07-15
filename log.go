package gproxy

import (
	"log"
	"os"
)

var logger = log.New(os.Stdout, "[gproxy] ", log.Ltime|log.Lshortfile)
