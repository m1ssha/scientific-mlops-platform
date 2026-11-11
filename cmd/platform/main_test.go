package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeparatePaths(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	assets := filepath.Join(root, "assets")
	for _, p := range []string{store, assets, filepath.Join(store, "nested")} {
		if e := os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := separatePaths(store, assets); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{store, root, filepath.Join(store, "nested")} {
		if e := separatePaths(store, p); e == nil {
			t.Fatalf("accepted overlap %s", p)
		}
	}
	link := filepath.Join(root, "link")
	if e := os.Symlink(store, link); e != nil {
		t.Fatal(e)
	}
	if e := separatePaths(store, link); e == nil {
		t.Fatal("accepted symlink to store")
	}
}
