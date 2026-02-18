package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
)

type builtinCompleter struct {
	builtins []string
}

func (c *builtinCompleter) Do(line []rune, pos int) ([][]rune, int) {
	input := string(line[:pos])
	fields := strings.Fields(input)

	if len(fields) == 0 {
		fmt.Print("\a")
		return nil, 0
	}

	current := fields[len(fields)-1]

	if len(fields) > 1 {
		return nil, 0
	}

	var candidates [][]rune

	for _, cmd := range c.builtins {
		if strings.HasPrefix(cmd, current) {
			suffix := cmd[len(current):] + " "
			candidates = append(candidates, []rune(suffix))
		}
	}

	pathEnv := os.Getenv("PATH")
	dirs := strings.Split(pathEnv, ":")

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			if strings.HasPrefix(name, current) {
				fullPath := dir + "/" + name
				info, err := os.Stat(fullPath)
				if err != nil {
					continue
				}

				if info.Mode()&0111 != 0 {
					suffix := name[len(current):] + " "
					candidates = append(candidates, []rune(suffix))
				}
			}
		}
	}

	if len(candidates) == 0 {
		fmt.Print("\a")
		return nil, 0
	}

	return candidates, len(current)
}

func main() {

	var builtins []string
	for cmd := range known_commands {
		builtins = append(builtins, cmd)
	}

	completer := &builtinCompleter{builtins: builtins}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "$ ",
		AutoComplete: completer,
		EOFPrompt:    "exit",
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading input", err)
		os.Exit(1)
	}
	defer rl.Close()

	var multi_string strings.Builder
	var in_new_line bool
	var command string
	var args []string
	var needs_more bool

NextPrompt:
	for {

	Input:
		for {

			if in_new_line {
				rl.SetPrompt(". ")
			} else {
				rl.SetPrompt("$ ")
			}

			line, err := rl.Readline()

			if err == readline.ErrInterrupt {
				multi_string.Reset()
				in_new_line = false
				command = ""
				fmt.Println()
				continue NextPrompt
			}

			if err == io.EOF {
				fmt.Println("exit")
				return
			}

			multi_string.WriteString(line)
			multi_string.WriteByte('\n')

			command, args, needs_more = parse_command(multi_string.String())

			if needs_more {
				in_new_line = true
				continue Input
			}

			in_new_line = false
			multi_string.Reset()
			break Input

		}

		switch command {

		case "":
			fmt.Println()
			lastExitCode = 0

		default:
			handle_command(command, args)
		}

	}
}

type commands map[string]func(args ...string)

var known_commands commands

func init() {
	known_commands = commands{
		"echo": func(args ...string) { fmt.Println(strings.Join(args, " ")); lastExitCode = 0 },

		"exit": func(args ...string) { os.Exit(0); lastExitCode = 0 },

		"type": func(args ...string) {
			if len(args) == 0 {
				lastExitCode = 0
				fmt.Println()
				return
			}
			for _, v := range args {
				if _, exists := get_method_bound_to_command(v); exists {
					lastExitCode = 0
					fmt.Println(v + " is a shell builtin")
				} else {
					path, err := resolve_command(v)
					if err != nil {
						lastExitCode = 1
						fmt.Println(v + ": not found")
					} else {
						lastExitCode = 0
						fmt.Println(v + " is " + path)
					}
				}
			}
		},

		"pwd": func(args ...string) {
			if current_dir, err := os.Getwd(); err != nil {
				fmt.Fprintln(os.Stderr, "pwd:", err)
				lastExitCode = 1
			} else {
				fmt.Println(current_dir)
				lastExitCode = 0
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
					lastExitCode = 1
					fmt.Fprintln(os.Stderr, "cd:", args[0]+":", "Error finding HOME variable")
					return
				}
				path = dic + path[1:]
			}

			err := os.Chdir(path)
			if err != nil {
				lastExitCode = 1
				fmt.Fprintln(os.Stderr, "cd:", args[0]+":", "No such file or directory")
				return
			}
			handle_output("ls", nil, "", 1, true)
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
	var arguments []string
	var output_file string
	last_position := len(args) - 1
	var num int
	var shouldAppend bool

loop:
	for i := range args {
		switch args[i] {

		case ">", "1>":
			num = 1
			shouldAppend = false

		case "2>":
			num = 2
			shouldAppend = false

		case "&>":
			num = 3
			shouldAppend = false

		case ">>", "1>>":
			shouldAppend = true
			num = 1

		case "2>>":
			shouldAppend = true
			num = 2

		case "&>>":
			shouldAppend = true
			num = 3

		default:
			continue
		}

		if i >= last_position {
			fmt.Println("parse error near `\\n'")
			return
		}

		arguments = args[:i]
		output_file = args[i+1]
		break loop
	}

	if arguments == nil {
		arguments = args
	}

	if comand_function, exists := get_method_bound_to_command(command); exists {
		handle_builtin_output(comand_function, arguments, output_file, num, shouldAppend)
		return
	}

	_, err := resolve_command(command)
	if err != nil {
		printResolveError(command, err)
		return
	}

	handle_output(command, arguments, output_file, num, shouldAppend)
}

var lastExitCode int

func handle_builtin_output(fn func(args ...string), arguments []string, outputFile string, num int, shouldAppend bool) {
	if outputFile == "" {
		fn(arguments...)
		return
	}

	var file *os.File
	var err error

	if !shouldAppend {
		file, err = os.Create(outputFile)
	} else {
		file, err = os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	}

	if err != nil {
		lastExitCode = 1
		fmt.Fprintln(os.Stderr, "error writing to file")
		return
	}
	defer file.Close()

	var oldStdOut *os.File
	var oldStdErr *os.File

	switch num {
	case 1:
		oldStdOut = os.Stdout
		os.Stdout = file
	case 2:
		oldStdErr = os.Stderr
		os.Stderr = file
	case 3:
		oldStdOut = os.Stdout
		os.Stdout = file
		oldStdErr = os.Stderr
		os.Stderr = file
	}

	defer func() {
		switch num {
		case 1:
			os.Stdout = oldStdOut
		case 2:
			os.Stderr = oldStdErr
		case 3:
			os.Stdout = oldStdOut
			os.Stderr = oldStdErr
		}
	}()

	fn(arguments...)
}

func handle_output(command string, args []string, outputFile string, num int, shouldAppend bool) {
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

	var file *os.File
	var err error

	if outputFile != "" {
		if !shouldAppend {
			file, err = os.Create(outputFile)
		} else {
			file, err = os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error writing to file")
			return
		}
		defer file.Close()

		switch num {

		case 1:
			cmd.Stdout = file
			cmd.Stderr = os.Stderr
		case 2:
			cmd.Stdout = os.Stdout
			cmd.Stderr = file
		case 3:
			cmd.Stderr = file
			cmd.Stdout = file
		}

	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err = cmd.Run()

	if err == nil {
		lastExitCode = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		lastExitCode = exitErr.ExitCode()
	} else {
		lastExitCode = 1
	}
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
	tokens []token
	state  lexar_state
}

type token struct {
	value string
	state token_state
}

type token_state struct {
	ShouldBeLiteral bool
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

	var expanded []string
	var val string
	for _, tok := range output.tokens {

		if tok.state.ShouldBeLiteral {
			val = tok.value
		} else {
			val = ExpandVars(tok)
		}

		if val != "" {
			expanded = append(expanded, val)
		}
	}

	command := expanded[0]
	arguments_slice := expanded[1:]

	return command, arguments_slice, false
}

func lex_input(arguments string) lexar_output {
	args := []token{}
	var current strings.Builder
	state := lexar_state{}
	arg_state := token_state{}

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

			if r == '$' || r == '`' {
				arg_state.ShouldBeLiteral = true
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

					if state.in_single_quotes {
						arg_state.ShouldBeLiteral = true
					}

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
					args = append(args, token{value: current.String(), state: arg_state})
					arg_state.ShouldBeLiteral = false
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
		args = append(args, token{value: current.String(), state: arg_state})
		arg_state.ShouldBeLiteral = false
	}

	return lexar_output{
		tokens: args,
		state:  state,
	}
}

func isCharValidInVar(r rune, pos int) bool {
	if pos == 0 {
		return unicode.IsLetter(r) || r == '_'
	}
	return unicode.IsDigit(r) || unicode.IsLetter(r) || r == '_'
}

func ExpandVars(s token) string {
	var out strings.Builder

	for i := 0; i < len(s.value); {
		if s.value[i] != '$' {
			out.WriteByte(s.value[i])
			i++
			continue
		}

		i++

		if i >= len(s.value) {
			out.WriteByte('$')
			break
		}

		start := i
		for i < len(s.value) {
			if isCharValidInVar(rune(s.value[i]), i-start) {
				i++
			} else {
				if unicode.IsDigit(rune(s.value[i])) {
					i++
				}
				break
			}
		}

		name := s.value[start:i]

		value := os.Getenv(name)
		out.WriteString(value)
	}

	return out.String()
}
