// Command admin manages user accounts from the server's command line. v1 has
// a single owner account and no sign-up page, so this is how it is created,
// and how a forgotten password is reset:
//
//	admin create-user -email owner@example.com -name "Khan"
//	admin reset-password -email owner@example.com
//
// The password is prompted for without echo, or read from the first line of
// standard input when that is not a terminal (for scripted provisioning). It
// is never accepted as a command-line argument, where it would be visible in
// the process list and shell history.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/config"
	"github.com/khansbikezone/bikezone-api/internal/domain"
)

const usage = `usage:
  admin create-user -email EMAIL -name NAME
  admin reset-password -email EMAIL`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing command\n%s", usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	emailAddr := fs.String("email", "", "the account's email address")
	name := fs.String("name", "", "display name (create-user)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *emailAddr == "" {
		return fmt.Errorf("-email is required\n%s", usage)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := app.New(ctx, cfg, config.NewLogger(cfg))
	if err != nil {
		return err
	}
	defer a.Close()
	if err := a.Migrate(ctx); err != nil { // a fresh install may not have run the API yet
		return err
	}

	switch args[0] {
	case "create-user":
		password, err := readPassword()
		if err != nil {
			return err
		}
		u, err := a.Auth.CreateUser(ctx, *emailAddr, *name, password, domain.RoleAdmin)
		if err != nil {
			return err
		}
		fmt.Printf("created admin %s (%s)\n", u.Email, u.ID)
	case "reset-password":
		password, err := readPassword()
		if err != nil {
			return err
		}
		if err := a.Auth.ResetPassword(ctx, *emailAddr, password); err != nil {
			return err
		}
		fmt.Printf("password reset for %s; all of its sessions were signed out\n", *emailAddr)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
	return nil
}

// readPassword prompts twice without echo on a terminal, or reads one line
// from a non-interactive standard input.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("the passwords do not match")
	}
	return string(first), nil
}
