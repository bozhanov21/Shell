package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	for {
		fmt.Print("$ ")

		raw_string, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading input", err)
			os.Exit(1)
		}

		trimmed := strings.TrimSpace(raw_string)
		command, args := parse_command(trimmed)

		switch command {

		case "":
			fmt.Println()

		default:
			comand_func, exists := known_commands[command]

			if exists {
				comand_func(args...)
			} else {
				fmt.Println(command + ": command not found")
			}

		}

	}
}

type commands map[string]func(args ...string)

var known_commands = commands{
	"echo": func(args ...string) { fmt.Println(strings.Join(args, " ")) },
	"exit": func(args ...string) { os.Exit(0) },
}

func parse_command(input string) (string, []string) {
	parts := strings.Split(input, " ")

	if len(parts) > 1 {
		return parts[0], parts[1:]
	}

	return parts[0], nil
}
