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
	var multi_string strings.Builder
	var in_new_line bool
	var command string
	var args []string
	var needs_more bool

	for {
		fmt.Print("$ ")

		for {
			if in_new_line {
				fmt.Print(". ")
			}

			raw_string, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error reading input", err)
				os.Exit(1)
			}

			multi_string.WriteString(raw_string)

			command, args, needs_more = parse_command(multi_string.String())

			if needs_more {
				in_new_line = true
				continue
			}

			in_new_line = false
			multi_string.Reset()
			break
		}

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

type lexar_state struct {
	in_single_quotes bool
	in_double_quotes bool
	escape_next      bool
}

type lexar_output struct {
	tokens []string
	state  lexar_state
}

func parse_command(input string) (string, []string, bool) {
	trimmed := strings.TrimSpace(input)

	if trimmed == "" {
		return "", nil, false
	}

	output := lex_input(trimmed)

	needs_more :=
		output.state.escape_next ||
			output.state.in_double_quotes ||
			output.state.in_single_quotes

	if needs_more {
		return "", nil, true
	}

	command := output.tokens[0]
	arguments_slice := output.tokens[1:]

	return command, arguments_slice, false
}

func lex_input(arguments string) lexar_output {
	var args []string
	var current strings.Builder
	state := lexar_state{}

	for _, r := range arguments {

		if state.escape_next {
			if r == '\n' {
				state.escape_next = false
				continue
			}
			if state.in_double_quotes {
				if r != '$' && r != '`' && r != '\\' && r != '"' {
					current.WriteRune('\\')
					state.escape_next = false
				}
			}
		}

		switch r {

		case '\\':
			if state.escape_next || state.in_single_quotes {
				current.WriteRune(r)
				state.escape_next = false
			} else {
				state.escape_next = true
			}

		case '"':
			if state.escape_next || state.in_single_quotes {
				current.WriteRune(r)
				state.escape_next = false
			} else {
				state.in_double_quotes = !state.in_double_quotes
			}

		case '\'':
			if state.escape_next {
				current.WriteRune(r)
				state.escape_next = false
			} else {
				if !state.in_double_quotes {
					state.in_single_quotes = !state.in_single_quotes
				} else {
					current.WriteRune(r)
				}
			}

		case ' ':
			if state.escape_next {
				current.WriteRune(r)
				state.escape_next = false
			} else {
				if state.in_single_quotes || state.in_double_quotes {
					current.WriteRune(r)
				} else if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			}

		default:
			current.WriteRune(r)
			if state.escape_next {
				state.escape_next = false
			}
		}

	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return lexar_output{
		tokens: args,
		state:  state,
	}
}

