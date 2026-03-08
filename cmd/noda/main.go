package main

import (
	"fmt"
	"os"
)

// Version is set at build time.
var Version = "0.0.1-dev"

func main() {
	fmt.Printf("noda %s\n", Version)
	os.Exit(0)
}
