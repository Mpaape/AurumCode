package main

import "os"

func main() {
	limpa("/tmp/aurum-demo")
}

func limpa(dir string) {
	_ = os.Getenv(dir)
}
