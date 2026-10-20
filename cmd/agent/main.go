package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wakecomputer/wakecomputer/internal/version"
)

func main() {
	cmd := "run"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("wake-agent", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "path to config.json")
	server := fs.String("server", "", "server URL (enroll)")
	code := fs.String("code", "", "one-time enrollment code (enroll)")
	insecure := fs.Bool("insecure", false, "skip TLS verification (LAN self-signed)")
	_ = fs.Parse(args)

	var err error
	switch cmd {
	case "enroll":
		err = enroll(*server, *code, *configPath, *insecure)
	case "run":
		err = runService(*configPath)
	case "install":
		err = installService(*configPath)
	case "uninstall":
		err = uninstallService()
	case "version":
		fmt.Printf("wake-agent %s (%s, %s)\n", version.Version, version.Commit, version.BuildTime)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		fmt.Fprintln(os.Stderr, "usage: wake-agent [enroll|run|install|uninstall|version] [-config path]")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
