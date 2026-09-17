// Command nilo-nethelper is the explicitly installed privileged network helper.
package main

import (
	"context"
	"flag"
	"fmt"
	"nilo.local/nilo/internal/nethelper"
	"os"
	"os/signal"
	"syscall"
)

type options struct {
	socket, state         string
	uid, gid              int
	raspberryPiRecovery   bool
	imageDefaultHostnames stringListFlag
}

type stringListFlag []string

func (values *stringListFlag) String() string { return fmt.Sprint([]string(*values)) }
func (values *stringListFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--verify-wireguard-transaction" {
		if err := nethelper.VerifyWireGuardTransaction(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "recover" {
		// One pass of independent recovery, for the root-owned timer.
		flags := flag.NewFlagSet("recover", flag.ExitOnError)
		state := flags.String("state-dir", "/var/lib/nilo-nethelper", "root-owned recovery directory")
		_ = flags.Parse(os.Args[2:])
		if flags.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "recover takes --state-dir only")
			os.Exit(2)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := recoverOnce(ctx, *state); err != nil {
			fmt.Fprintln(os.Stderr, "network recovery:", err)
			os.Exit(1)
		}
		return
	}
	var o options
	flag.StringVar(&o.socket, "socket", "/run/nilo/nethelper.sock", "restricted Unix socket")
	flag.StringVar(&o.state, "state-dir", "/var/lib/nilo-nethelper", "root-owned recovery directory")
	flag.IntVar(&o.uid, "allow-uid", -1, "non-root Bench Core service UID")
	flag.IntVar(&o.gid, "socket-gid", -1, "Bench Core service group")
	flag.BoolVar(&o.raspberryPiRecovery, "raspberry-pi-recovery", false, "enable BCM17 recovery button and BCM27 status LED")
	flag.Var(&o.imageDefaultHostnames, "image-default-hostname", "generic image hostname eligible for one-time personalization (repeatable)")
	flag.Parse()
	if flag.NArg() != 0 || o.uid <= 0 || o.gid < 0 {
		fmt.Fprintln(os.Stderr, "explicit non-root --allow-uid and --socket-gid required")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := serve(ctx, o); err != nil {
		fmt.Fprintln(os.Stderr, "network helper:", err)
		os.Exit(1)
	}
}
