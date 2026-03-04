package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
)

const serviceName = "greyproxy"

func installBinPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "greyproxy")
}

func handleInstall() {
	binDst := installBinPath()

	binSrc, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine current executable: %v\n", err)
		os.Exit(1)
	}
	binSrc, _ = filepath.EvalSymlinks(binSrc)

	fmt.Printf("This will:\n")
	fmt.Printf("  1. Copy %s -> %s\n", binSrc, binDst)
	fmt.Printf("  2. Install greyproxy as a systemd user service\n")
	fmt.Printf("  3. Start the service\n")
	fmt.Printf("\nProceed? [y/N] ")

	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return
	}

	// 1. Copy binary
	if err := copyBinary(binSrc, binDst); err != nil {
		fmt.Fprintf(os.Stderr, "error: copying binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary to %s\n", binDst)

	// 2. Register service
	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: "Greyproxy",
		Description: "Greyproxy network proxy service",
		Executable:  binDst,
		Option: service.KeyValue{
			"UserService": true,
		},
	}

	p := &program{}
	s, err := service.New(p, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := service.Control(s, "install"); err != nil {
		fmt.Fprintf(os.Stderr, "error: registering service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Registered systemd user service")

	// 3. Start service
	if err := service.Control(s, "start"); err != nil {
		fmt.Fprintf(os.Stderr, "error: starting service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Service started")
}

func handleUninstall() {
	binDst := installBinPath()

	fmt.Printf("This will:\n")
	fmt.Printf("  1. Stop the greyproxy service\n")
	fmt.Printf("  2. Remove the systemd user service\n")
	fmt.Printf("  3. Remove %s\n", binDst)
	fmt.Printf("\nProceed? [y/N] ")

	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return
	}

	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: "Greyproxy",
		Description: "Greyproxy network proxy service",
		Executable:  binDst,
		Option: service.KeyValue{
			"UserService": true,
		},
	}

	p := &program{}
	s, err := service.New(p, svcConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// 1. Stop service (ignore error — may already be stopped)
	_ = service.Control(s, "stop")
	fmt.Println("Service stopped")

	// 2. Unregister service
	if err := service.Control(s, "uninstall"); err != nil {
		fmt.Fprintf(os.Stderr, "error: removing service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Removed systemd user service")

	// 3. Remove binary
	if err := os.Remove(binDst); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: removing binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s\n", binDst)
}

func copyBinary(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
