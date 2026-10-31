package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lucasnevespereira/canape/internal/qrcode"
	"github.com/lucasnevespereira/canape/internal/server"
	"github.com/lucasnevespereira/canape/internal/store"
)

// The background dashboard leaves its pid and address here, and its output.
var (
	pidFile = func() string { return filepath.Join(filepath.Dir(store.Path()), "serve.pid") }
	logFile = func() string { return filepath.Join(filepath.Dir(store.Path()), "serve.log") }
)

func serveCommand(args []string) error {
	addr, detach, background := ":8080", false, false
	for _, a := range args {
		switch a {
		case "stop":
			return stopServe()
		case "-d", "--detach":
			detach = true
		case "--background": // the detached process itself, started by -d
			background = true
		default:
			addr = a
		}
	}
	switch {
	case background:
		return server.Run(addr, func(string, string) {})
	case detach:
		return detachServe(addr)
	}
	return server.Run(addr, func(name, link string) { printLogin(name, link, false) })
}

// detachServe starts the dashboard as its own process, waits until it
// answers, then prints the login link and gives the terminal back.
func detachServe(addr string) error {
	if running, ok := runningServe(); ok {
		name, link, err := server.Link(running)
		if err != nil {
			return err
		}
		printLogin(name, link, true)
		return nil
	}
	if answers(addr) {
		return fmt.Errorf("canape serve already runs on port %s, in another terminal window. Stop it there with Ctrl+C, or pick another port: canape serve -d :8081", addr[strings.LastIndex(addr, ":")+1:])
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logFile()), 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(logFile(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	c := exec.Command(exe, "serve", "--background", addr)
	c.Stdout, c.Stderr = logf, logf
	// Its own session, so closing the terminal doesn't stop it. It's still
	// started from the terminal app, which keeps the Accessibility permission
	// that computer keys need.
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	exited := make(chan struct{})
	go func() { c.Wait(); close(exited) }()
	for deadline := time.Now().Add(10 * time.Second); !answers(addr); {
		select {
		case <-exited:
			out, _ := os.ReadFile(logFile())
			return errors.New(strings.TrimPrefix(strings.TrimSpace(string(out)), "canape: "))
		case <-time.After(200 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			c.Process.Kill()
			return fmt.Errorf("the dashboard didn't start. Its output is in %s", logFile())
		}
	}
	if err := os.WriteFile(pidFile(), fmt.Appendf(nil, "%d %s\n", c.Process.Pid, addr), 0o600); err != nil {
		return err
	}
	name, link, err := server.Link(addr)
	if err != nil {
		return err
	}
	printLogin(name, link, true)
	return nil
}

func stopServe() error {
	pid, _, _ := readPidFile()
	_, running := runningServe()
	os.Remove(pidFile())
	if !running {
		fmt.Println("canape serve isn't running in the background.")
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	fmt.Println("Stopped the dashboard.")
	return nil
}

// runningServe returns the address of the background dashboard, if one runs.
// After a restart the pid file is stale and its pid may belong to something
// else, so the dashboard has to answer too.
func runningServe() (string, bool) {
	pid, addr, ok := readPidFile()
	if !ok || syscall.Kill(pid, 0) != nil || !answers(addr) {
		return "", false
	}
	return addr, true
}

func readPidFile() (pid int, addr string, ok bool) {
	b, err := os.ReadFile(pidFile())
	if err != nil {
		return 0, "", false
	}
	p, a, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	pid, err = strconv.Atoi(p)
	return pid, a, err == nil && a != ""
}

// answers checks that a canape dashboard serves on addr.
func answers(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/manifest.webmanifest")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func printLogin(name, link string, background bool) {
	fmt.Printf("canape is running for home %q.\n\nScan with your phone to open the web dashboard:\n\n", name)
	if code, err := qrcode.Terminal(link); err == nil {
		fmt.Print(code)
	}
	fmt.Printf("\nOr open %s on a device on the same Wi-Fi.\n", link)
	fmt.Println("The link logs the device in, so only use it on your own devices.")
	if background {
		fmt.Println("It runs in the background, so you can close this window. Stop it with: canape serve stop")
	} else {
		fmt.Println("Ctrl+C to stop. To keep it running without this window: canape serve -d")
	}
}
