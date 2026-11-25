package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"readerhub/internal/buildinfo"
	"readerhub/internal/bundle"
	"readerhub/internal/client"
	"readerhub/internal/config"
	"readerhub/internal/document"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "readerhub-cli:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Println("ReaderHub CLI\n  readerhub-cli configure --hub http://<Hub-IP>:8848\n  readerhub-cli add <article.md> [--project name] [--title title]\n  readerhub-cli --version\nToken: configure reads from stdin, or use READERHUB_TOKEN. Hub can also be set with READERHUB_URL.")
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println("readerhub-cli", buildinfo.Version)
		return nil
	}
	path := config.DefaultClientPath()
	c, err := config.ReadClient(path)
	if err != nil {
		return err
	}
	if c.DeviceID == "" {
		c.DeviceID = document.NewID()
	}
	if c.Machine == "" {
		c.Machine, _ = os.Hostname()
	}
	if args[0] == "configure" {
		flags := flag.NewFlagSet("configure", flag.ContinueOnError)
		hubURL := flags.String("hub", c.HubURL, "Hub URL")
		name := flags.String("name", c.Machine, "device display name")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if err := validURL(*hubURL); err != nil {
			return err
		}
		c.HubURL = strings.TrimRight(*hubURL, "/")
		c.Machine = *name
		if token := os.Getenv("READERHUB_TOKEN"); token != "" {
			c.Token = token
		} else {
			fmt.Fprint(os.Stderr, "Hub access token (input is not logged): ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && strings.TrimSpace(line) == "" {
				return fmt.Errorf("read token: %w", err)
			}
			c.Token = strings.TrimSpace(line)
		}
		if c.Token == "" {
			return fmt.Errorf("access token is required")
		}
		if err := config.WriteClient(path, c); err != nil {
			return err
		}
		fmt.Println("Configured", c.HubURL, "for", c.Machine)
		return nil
	}
	if args[0] != "add" {
		return fmt.Errorf("unknown command %q; use --help", args[0])
	}
	input := args[1:]
	file := ""
	if len(input) > 0 && !strings.HasPrefix(input[0], "-") {
		file = input[0]
		input = input[1:]
	}
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	project := flags.String("project", "", "source project")
	title := flags.String("title", "", "display title")
	hubURL := flags.String("hub", envOr("READERHUB_URL", c.HubURL), "Hub URL")
	if err := flags.Parse(input); err != nil {
		return err
	}
	if file == "" && flags.NArg() == 1 {
		file = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return fmt.Errorf("provide exactly one Markdown file")
	}
	if file == "" {
		return fmt.Errorf("provide one Markdown file")
	}
	if err := validURL(*hubURL); err != nil {
		return err
	}
	token := envOr("READERHUB_TOKEN", c.Token)
	if token == "" {
		return fmt.Errorf("configure an access token first")
	}
	if err := config.WriteClient(path, c); err != nil {
		return err
	}
	if *project == "" {
		absolute, _ := filepath.Abs(file)
		*project = filepath.Base(filepath.Dir(absolute))
	}
	b, err := bundle.Collect(context.Background(), file, *title, document.Source{DeviceID: c.DeviceID, Machine: c.Machine, Project: *project})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Collected article and %d image references (%d unique files)\n", len(b.Upload.Assets), len(b.Files))
	receipt, err := (client.Client{URL: *hubURL, Token: token}).Submit(context.Background(), b)
	if err != nil {
		return err
	}
	receipt.URL = strings.TrimRight(*hubURL, "/") + receipt.URL
	return json.NewEncoder(os.Stdout).Encode(receipt)
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return fmt.Errorf("provide a valid Hub http(s) origin")
	}
	return nil
}
