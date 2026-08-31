package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/buildinfo"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	if len(arguments) == 1 && arguments[0] == "version" {
		if err := json.NewEncoder(os.Stdout).Encode(buildinfo.Current()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if len(arguments) == 3 && arguments[0] == "serve-foundation-check" && arguments[1] == "--data-dir" {
		application, err := app.New(context.Background(), app.Options{
			DataDir: arguments[2], BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		health := struct {
			Status     string `json:"status"`
			DataDir    string `json:"data_dir"`
			Reconciled bool   `json:"reconciled"`
		}{Status: "ok", DataDir: application.Configuration().DataDir, Reconciled: true}
		if err := json.NewEncoder(os.Stdout).Encode(health); err != nil {
			_ = application.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := application.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(os.Stderr, "usage: nilo version | nilo serve-foundation-check --data-dir <absolute-path>")
	return 2
}
