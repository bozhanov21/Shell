package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
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
		command, args, err := parse_command(trimmed)

		if err != nil {
			log.Fatalln(err)
		}

		comand_func, exists := known_commands[command]

		if exists {
			comand_func(args...)
		} else {
			fmt.Println(command + ": command not found")
		}

	}
}

type commands map[string]func(args ...string)

var known_commands = commands{
	"echo": func(args ...string) { fmt.Println(strings.Join(args, " ")) },
	"exit": func(args ...string) { os.Exit(0) },
}

func parse_command(input string) (string, []string, error) {
	parts := strings.Split(input, " ")

	if parts[0] == "" {
		return "", nil, errors.New("Failed parsing the command")
	}

	if len(parts) > 1 {
		return parts[0], parts[1:], nil
	}

	return parts[0], nil, nil

}
