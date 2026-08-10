package main

import (
	"flag"
	"fmt"
	"os"

	"ossfind/internal/state"
)

func runResetState(args []string) {
	fs := flag.NewFlagSet("reset-state", flag.ExitOnError)
	statePath := fs.String("state-path", "", "override the local state file location (default: ~/.config/ossfind/state.json)")
	yes := fs.Bool("yes", false, "confirm deletion (required)")
	fs.Parse(args)

	path := *statePath
	if path == "" {
		p, err := state.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error resolving state path: %v\n", err)
			os.Exit(1)
		}
		path = p
	}

	if !*yes {
		fmt.Printf("This will permanently delete local history at %s (seen issues + run streak).\n", path)
		fmt.Println("Re-run with -yes to confirm.")
		return
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No state file found — nothing to reset.")
			return
		}
		fmt.Fprintf(os.Stderr, "error removing state file: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Local history cleared.")
}
