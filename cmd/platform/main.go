package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/m1ssha/scientific-mlops-platform/internal/api"
	"github.com/m1ssha/scientific-mlops-platform/internal/web"
)

func main() {
	root := flag.String("store", "", "existing core store directory")
	webDir := flag.String("web-dir", "", "optional production web build directory")
	address := flag.String("listen", "127.0.0.1:8080", "literal loopback IP and port")
	flag.Parse()
	if *root == "" || flag.NArg() != 0 {
		log.Fatal("--store is required; no positional arguments")
	}
	listener, err := api.Listen(*address)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	handler, err := api.New(context.Background(), *root, listener.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	if *webDir != "" {
		if err = separatePaths(*root, *webDir); err != nil {
			log.Fatal(err)
		}
		handler, err = web.New(*webDir, listener.Addr().String(), handler)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("listening on http://%s", listener.Addr())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second}
	log.Fatal(server.Serve(listener))
}

// separatePaths prevents a static build from including any part of the store.
func separatePaths(store, webDir string) error {
	s, err := filepath.EvalSymlinks(store)
	if err != nil {
		return err
	}
	s, err = filepath.Abs(s)
	if err != nil {
		return err
	}
	w, err := filepath.EvalSymlinks(webDir)
	if err != nil {
		return err
	}
	w, err = filepath.Abs(w)
	if err != nil {
		return err
	}
	contains := func(parent, child string) bool {
		r, e := filepath.Rel(parent, child)
		return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
	}
	if contains(s, w) || contains(w, s) {
		return fmt.Errorf("web directory and store must not overlap")
	}
	return nil
}
