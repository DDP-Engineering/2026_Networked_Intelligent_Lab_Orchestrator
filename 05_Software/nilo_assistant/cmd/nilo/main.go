package main

import (
	"encoding/json"
	"fmt"
	"os"

	"nilo.local/nilo/internal/buildinfo"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		if err := json.NewEncoder(os.Stdout).Encode(buildinfo.Current()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	fmt.Fprintln(os.Stderr, "usage: nilo version")
	os.Exit(2)
}
