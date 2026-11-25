package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"readerhub/internal/buildinfo"
	"readerhub/internal/config"
	"readerhub/internal/httpapi"
	"readerhub/internal/hub"
	"readerhub/internal/storage"
	webassets "readerhub/web"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Println("ReaderHub\n  readerhub serve [--listen 127.0.0.1:8848] [--data ~/ReaderHubData]\n  readerhub token [--data ~/ReaderHubData]\n  readerhub --version")
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println("readerhub", buildinfo.Version)
		return nil
	}
	if args[0] != "serve" && args[0] != "token" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	home, _ := os.UserHomeDir()
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	data := flags.String("data", filepath.Join(home, "ReaderHubData"), "persistent data directory")
	listen := flags.String("listen", "127.0.0.1:8848", "HTTP listen address")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	root, err := filepath.Abs(*data)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if args[0] == "token" {
		token, err := os.ReadFile(filepath.Join(root, "access-token"))
		if err != nil {
			return err
		}
		fmt.Println(string(token))
		return nil
	}
	lock, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("data directory is already in use: %w", err)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	store, err := storage.Open(root)
	if err != nil {
		return err
	}
	defer store.Close()
	token, err := config.HubToken(root)
	if err != nil {
		return err
	}
	files, err := fs.Sub(webassets.Files, "dist")
	if err != nil {
		return err
	}
	api := (&httpapi.Server{App: &hub.App{Store: store}, Token: token, Web: files}).Handler()
	server := httpapi.HTTPServer(*listen, api)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("ReaderHub %s listening on %s; data=%s; access token file=%s", buildinfo.Version, listener.Addr(), root, filepath.Join(root, "access-token"))
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
