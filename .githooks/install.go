//go:build ignore

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	rootDir, err := filepath.Abs(filepath.Join(".", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving root dir: %v\n", err)
		os.Exit(1)
	}

	srcPath := filepath.Join(rootDir, ".githooks", "pre-commit")
	dstDir := filepath.Join(rootDir, ".git", "hooks")
	dstPath := filepath.Join(dstDir, "pre-commit")

	src, err := os.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening source hook: %v\n", err)
		os.Exit(1)
	}
	defer src.Close()

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating hooks dir: %v\n", err)
		os.Exit(1)
	}

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating destination hook: %v\n", err)
		os.Exit(1)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		fmt.Fprintf(os.Stderr, "error copying hook: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("pre-commit hook installed successfully")
}
