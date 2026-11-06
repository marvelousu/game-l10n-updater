package main

import (
	"os"

	"github.com/marvelousu/game-l10n-updater/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
