package commands

import "fmt"

// clearScreen sends ANSI escape codes to clear the terminal and move the cursor
// to the top-left corner. Used by --pane mode commands in the zellij dashboard.
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}
