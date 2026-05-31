package main

import (
	"os"

	"github.com/spencerrais/utree/internal/app"
)

func main() {
	os.Exit(app.App{Stdout: os.Stdout, Stderr: os.Stderr}.Run(os.Args[1:]))
}
