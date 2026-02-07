package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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

		command, args := parse_command(raw_string)

		switch command {

		case "":
			fmt.Println()

		case "type":
			if len(args) == 0 {
				fmt.Println()
				break
			}
			handle_type_case(args[0])

		default:
			handle_command(command, args)
		}

	}
}

type commands map[string]func(args ...string)

var known_commands commands

func init() {
	known_commands = commands{
		"exit": func(args ...string) { os.Exit(0) },

		"echo": func(args ...string) { fmt.Println(strings.Join(args, " ")) },

		"type": func(args ...string) { /* returns the type (done separately) */ },

		"pwd": func(args ...string) {
			if current_dir, err := os.Getwd(); err != nil {
				fmt.Fprintln(os.Stderr, "pwd:", err)
			} else {
				fmt.Println(current_dir)
			}
		},

		"cd": func(args ...string) {
			var path string

			if len(args) == 0 {
				path = "~"
			} else {
				path = args[0]
			}

			if strings.HasPrefix(path, "~") {
				dic, err := os.UserHomeDir()
				if err != nil {
					fmt.Fprintln(os.Stderr, "cd:", args[0]+":", "Error finding HOME variable")
					return
				}
				path = dic + path[1:]
			}

			err := os.Chdir(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "cd:", args[0]+":", "No such file or directory")
				return
			}
			handle_output("ls")
		},
	}
}

var (
	ErrNotFound   = errors.New("not found")
	ErrPermission = errors.New("permission denied")
)

func printResolveError(cmd string, err error) {
	switch err {

	case ErrNotFound:
		fmt.Println(cmd + ": command not found")

	case ErrPermission:
		fmt.Println(cmd + ": permission denied")

	default:
		fmt.Println(cmd + ": error")
	}
}

func handle_command(command string, args []string) {
	if comand_function, exists := get_method_bound_to_command(command); exists {
		comand_function(args...)
		lastExitCode = 0
		return
	}

	_, err := resolve_command(command)
	if err != nil {
		printResolveError(command, err)
		return
	}

	handle_output(command, args...)
}

var lastExitCode int

func handle_output(command string, args ...string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	go func() {
		select {

		case <-sig:
			cancel()

		case <-ctx.Done():

		}
	}()

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if err == nil {
		lastExitCode = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		lastExitCode = exitErr.ExitCode()
	} else {
		lastExitCode = 1
	}
}

func handle_type_case(cmd string) {
	if _, exists := get_method_bound_to_command(cmd); exists {
		fmt.Println(cmd + " is a shell builtin")
		return
	}

	path, err := resolve_command(cmd)
	if err != nil {
		fmt.Println(cmd + ": not found")
		return
	}

	fmt.Println(cmd + " is " + path)
}

func resolve_command(cmd string) (string, error) {
	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", ErrNotFound
	}
	return path, nil
}

func get_method_bound_to_command(command string) (func(args ...string), bool) {
	comand_func, exists := known_commands[command]
	return comand_func, exists
}

func parse_command(input string) (string, []string) {
	trimmed := strings.TrimSpace(input)

	if trimmed == "" {
		return "", nil
	}

	command, arguments, exists := strings.Cut(trimmed, " ")

	if !exists {
		return command, nil
	}

	var args []string
	var current strings.Builder
	inQuote := false

	for _, r := range arguments {
		switch r {
		case '\'':
			inQuote = !inQuote

		case ' ':
			if inQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}

		default:
			current.WriteRune(r)
		}

	}
	// end of input
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return command, args
}
