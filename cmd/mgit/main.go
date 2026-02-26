package main

import (
	"context"
	"os"

	"mgit/internal/cli"
)

func main() {
	app := cli.New(os.Stdin, os.Stdout, os.Stderr)
	code := app.Run(context.Background(), os.Args[1:])
	os.Exit(code)
}
