package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	flag "github.com/spf13/pflag"
)

func handleServiceCommand(args []string) {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	fs.ParseErrorsWhitelist.UnknownFlags = false
	var (
		cfgFlag  string
		nameFlag string
	)
	fs.StringVarP(&cfgFlag, "config", "C", "", "configuration file")
	fs.StringVar(&nameFlag, "name", "greyproxy", "service name")
	fs.Usage = printServiceUsage

	if len(args) == 0 {
		printServiceUsage()
		os.Exit(1)
	}

	action := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		os.Exit(1)
	}

	svcConfig := &service.Config{
		Name:        nameFlag,
		DisplayName: "Greyproxy",
		Description: "Greyproxy network proxy service",
		Arguments:   []string{"serve"},
		Option: service.KeyValue{
			"UserService": true,
		},
	}

	// On install, pass -C to the service config so the service manager
	// starts the binary with the correct config path.
	if action == "install" && cfgFlag != "" {
		absPath, err := filepath.Abs(cfgFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: resolving config path: %v\n", err)
			os.Exit(1)
		}
		svcConfig.Arguments = []string{"serve", "-C", absPath}
	}

	p := &program{}
	s, err := service.New(p, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch action {
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(s, action); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("service %s: %s succeeded\n", nameFlag, action)

	case "status":
		status, err := s.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		switch status {
		case service.StatusRunning:
			fmt.Printf("service %s: running\n", nameFlag)
		case service.StatusStopped:
			fmt.Printf("service %s: stopped\n", nameFlag)
		default:
			fmt.Printf("service %s: unknown\n", nameFlag)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown action: %s\n", action)
		printServiceUsage()
		os.Exit(1)
	}
}

func printServiceUsage() {
	fmt.Fprintf(os.Stderr, `greyproxy %s (%s %s/%s)

Usage: greyproxy service <action> [flags]

Actions:
  install     Register greyproxy as an OS service
  uninstall   Remove the OS service registration
  start       Start the service
  stop        Stop the service
  restart     Restart the service
  status      Show service status

Flags:
  -C, --config string  Configuration file (optional, only used with install)
      --name string    Service name (default "greyproxy")
`, version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
