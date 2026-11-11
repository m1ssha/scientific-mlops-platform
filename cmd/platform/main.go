package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/m1ssha/scientific-mlops-platform/internal/api"
)

func main() {
	root := flag.String("store", "", "existing core store directory")
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
	log.Printf("listening on http://%s", listener.Addr())
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second}
	log.Fatal(server.Serve(listener))
}
