package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("AurumCode CLI")
		fmt.Println("Usage: aurumcode-cli <command> [args...]")
		os.Exit(1)
	}

	command := os.Args[1]
	
	switch command {
	case "version":
		fmt.Println("AurumCode CLI v0.1.0")
	case "help":
		fmt.Println("Available commands:")
		fmt.Println("  version  - Show version information")
		fmt.Println("  help     - Show this help message")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

