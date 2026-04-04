package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	commandFile := flag.String("command-file", "", "write add-window command here instead of running it")
	returnCommand := flag.String("return-command", "", "append this command to the command file after add-window")
	switchCommand := flag.String("switch-command", "", "write this command to the command file when switching to another tool")
	flag.Parse()

	initialSessID, initialWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "laneboard: %v\n", err)
		os.Exit(1)
	}

	m, err := newModel(initialSessID, initialWinID, *commandFile, *returnCommand, *switchCommand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "laneboard: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "laneboard: %v\n", err)
		os.Exit(1)
	}
}
