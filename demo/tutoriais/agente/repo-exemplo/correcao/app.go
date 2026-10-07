package main

import "os"

func main() {
	if err := limpa("/tmp/aurum-demo"); err != nil {
		os.Exit(1)
	}
}

func limpa(dir string) error {
	return os.RemoveAll(dir)
}
