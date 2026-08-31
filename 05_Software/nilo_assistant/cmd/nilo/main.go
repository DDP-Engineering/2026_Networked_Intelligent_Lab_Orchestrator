package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/httpapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, arguments []string) int {
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
	if len(arguments) == 4 && arguments[0] == "development" && arguments[1] == "issue-token" && arguments[2] == "--data-dir" {
		application, err := app.New(ctx, app.Options{
			DataDir: arguments[3], BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		issued, err := application.IssueDevelopmentToken(ctx)
		if err != nil {
			_ = application.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		output := struct {
			TokenID   string    `json:"token_id"`
			UserID    string    `json:"user_id"`
			Token     string    `json:"token"`
			CreatedAt time.Time `json:"created_at"`
		}{TokenID: issued.Token.ID, UserID: issued.UserID, Token: issued.Token.Plaintext, CreatedAt: issued.Token.CreatedAt}
		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
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
	if len(arguments) > 0 && arguments[0] == "serve" {
		return runServe(ctx, arguments[1:])
	}

	writeUsage()
	return 2
}

func runServe(ctx context.Context, arguments []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profile := flags.String("profile", "", "runtime profile")
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	bindAddress := flags.String("bind", "", "loopback listener address")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *profile != "simulation" || *dataDir == "" || *bindAddress == "" {
		fmt.Fprintln(os.Stderr, "serve requires --profile simulation, --data-dir, and --bind")
		return 2
	}
	if err := httpapi.ValidateSimulationBind(*bindAddress); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	application, err := app.New(ctx, app.Options{
		DataDir: *dataDir, BindAddress: *bindAddress, Now: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := application.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	handler, err := httpapi.NewHandler(httpapi.HandlerOptions{
		Tokens: application.TokenRepository(), Profile: *profile, Build: buildinfo.Current(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = httpapi.Serve(ctx, httpapi.ServeOptions{
		Address:         *bindAddress,
		Handler:         handler,
		ShutdownTimeout: 5 * time.Second,
		OnListening: func(address net.Addr) {
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Status  string `json:"status"`
				Address string `json:"address"`
				Profile string `json:"profile"`
			}{Status: "listening", Address: address.String(), Profile: *profile})
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func writeUsage() {
	fmt.Fprintln(os.Stderr, "usage: nilo version | nilo serve-foundation-check --data-dir <absolute-path> | nilo development issue-token --data-dir <absolute-path> | nilo serve --profile simulation --data-dir <absolute-path> --bind <loopback-address>")
}
