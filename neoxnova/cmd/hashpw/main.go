// Command hashpw prints an encoded password hash for seeding users:
//
//	go run ./cmd/hashpw 'my-dev-password'
package main

import (
	"fmt"
	"os"

	"neoxnova/internal/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hashpw <password>")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
