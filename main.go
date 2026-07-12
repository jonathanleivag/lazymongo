package main

import (
	"fmt"
	"os"

	"github.com/jonathanleivag/lazymongo/internal/config"
	"github.com/jonathanleivag/lazymongo/internal/mongo"
	"github.com/jonathanleivag/lazymongo/internal/tui"
)

func main() {
	client := mongo.NewRealClient()

	if len(os.Args) < 2 {
		if err := tui.Run(client, nil); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	conn, err := config.ResolveConnection(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := tui.Run(client, &conn); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
