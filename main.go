// Command ossfind searches GitHub for recently opened, unassigned issues
// that match a set of languages and a rough experience level (beginner /
// intermediate / advanced), scores them by rough difficulty, and remembers
// what it's shown you so repeat runs surface fresh issues instead of the
// same handful on a loop.
//
// Subcommands:
//
//	ossfind [find flags...]   find matching issues (default if no subcommand given)
//	ossfind stats             show your local contribution-hunting stats/streak
//	ossfind reset-state -yes  wipe local history (seen issues + run log)
//
// Examples:
//
//	ossfind -languages Go,Python -level beginner
//	ossfind -languages Go -level beginner -sort difficulty
//	ossfind -languages Go -level beginner -cooldown 3
//	ossfind stats
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "stats":
			runStats(os.Args[2:])
			return
		case "reset-state":
			runResetState(os.Args[2:])
			return
		case "find":
			runFind(os.Args[2:])
			return
		case "-h", "--help", "help":
			fmt.Println("usage: ossfind [find flags...] | ossfind stats | ossfind reset-state -yes")
			fmt.Println("run `ossfind -h` (no subcommand) to see all find flags")
			return
		}
	}
	// No recognized subcommand -- treat all args as find flags (backward
	// compatible with the original single-command interface).
	runFind(os.Args[1:])
}
