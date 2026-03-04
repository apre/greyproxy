package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	flag "github.com/spf13/pflag"
)

const serviceName = "greyproxy"

func parseInstallFlags(args []string) (force bool) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.BoolVarP(&force, "force", "f", false, "skip confirmation prompts")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return
}

func installBinPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "greyproxy")
}

func newServiceControl() (service.Service, error) {
	binDst := installBinPath()
	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: "Greyproxy",
		Description: "Greyproxy network proxy service",
		Executable:  binDst,
		Arguments:   []string{"serve"},
		Option: service.KeyValue{
			"UserService": true,
		},
	}
	return service.New(&program{}, svcConfig)
}

func isInstalled() bool {
	_, err := os.Stat(installBinPath())
	return err == nil
}

func handleInstall(args []string) {
	force := parseInstallFlags(args)
	binDst := installBinPath()

	binSrc, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine current executable: %v\n", err)
		os.Exit(1)
	}
	binSrc, _ = filepath.EvalSymlinks(binSrc)

	if isInstalled() {
		handleReinstall(binSrc, binDst, force)
		return
	}

	fmt.Printf("This will:\n")
	fmt.Printf("  1. Copy %s -> %s\n", binSrc, binDst)
	fmt.Printf("  2. Install greyproxy as a systemd user service\n")
	fmt.Printf("  3. Start the service\n")

	if !force {
		fmt.Printf("\nProceed? [y/N] ")
		if !askConfirm() {
			return
		}
	}

	freshInstall(binSrc, binDst)
}

func handleReinstall(binSrc, binDst string, force bool) {
	fmt.Printf("An existing installation was found at %s\n", binDst)
	fmt.Printf("\nThis will:\n")
	fmt.Printf("  1. Stop the running service\n")
	fmt.Printf("  2. Remove the current service registration\n")
	fmt.Printf("  3. Replace the binary with %s\n", binSrc)
	fmt.Printf("  4. Re-register the systemd user service\n")
	fmt.Printf("  5. Start the service\n")

	if !force {
		fmt.Printf("\nUpdate existing installation? [y/N] ")
		if !askConfirm() {
			return
		}
	}

	s, err := newServiceControl()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// 1. Stop service (ignore error — may already be stopped)
	_ = service.Control(s, "stop")
	fmt.Println("Service stopped")

	// 2. Unregister old service (ignore error — may not be registered)
	_ = service.Control(s, "uninstall")
	fmt.Println("Removed old service registration")

	// 3-5. Fresh install (copy binary, register, start)
	freshInstall(binSrc, binDst)
}

func freshInstall(binSrc, binDst string) {
	// Copy binary
	if err := copyBinary(binSrc, binDst); err != nil {
		fmt.Fprintf(os.Stderr, "error: copying binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Installed binary to %s\n", binDst)

	// Register service
	s, err := newServiceControl()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := service.Control(s, "install"); err != nil {
		fmt.Fprintf(os.Stderr, "error: registering service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Registered systemd user service")

	// Start service
	if err := service.Control(s, "start"); err != nil {
		fmt.Fprintf(os.Stderr, "error: starting service: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Service started")
}

func askConfirm() bool {
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return false
	}
	return true
}

func handleUninstall(args []string) {
	force := parseInstallFlags(args)
	binDst := installBinPath()

	fmt.Printf("This will:\n")
	fmt.Printf("  1. Stop the greyproxy service\n")
	fmt.Printf("  2. Remove the systemd user service\n")
	fmt.Printf("  3. Remove %s\n", binDst)

	if !force {
		fmt.Printf("\nProceed? [y/N] ")
		if !askConfirm() {
			return
		}
	}

	s, err := newServiceControl()
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
