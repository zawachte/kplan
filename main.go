package main

import (
	"fmt"
	"os"

	"github.com/zawachte/kplan/internal/cli"
)

func main() {
	if err := cli.New().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
