package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func main() {
	fileName := os.Args[1]

	fileContent, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	r := bytes.NewReader(fileContent)
	script, err := syntax.NewParser().Parse(r, filepath.Base(fileName))
	if err != nil {
		panic(err)
	}

	var (
		execArgs     = make(chan []string)
		execHandlers = make(chan interp.ExecHandlerFunc)
		scriptError  = make(chan error)
	)

	go func() {
		defer close(execArgs)
		defer close(scriptError)
		runner, err := interp.New(
			interp.StdIO(nil, os.Stdout, os.Stdout),
			interp.ExecHandler(func(ctx context.Context, args []string) error {
				execArgs <- args
				fn := <-execHandlers
				return fn(ctx, args)
			}),
		)
		if err != nil {
			return
		}
		ctx := context.Background()
		err = runner.Run(ctx, script)
		if err != nil {
			scriptError <- err
		}
	}()

	fallThoughHandler := interp.DefaultExecHandler(time.Second)
	var (
		argsDone, scriptErrDone bool
	)
	for !argsDone || !scriptErrDone {
		select {
		case args, ok := <-execArgs:
			if !ok {
				argsDone = true
				execArgs = nil
				continue
			}
			switch getInterceptionOption(args) {
			case CallOptionMock:
				go getAndSendExecFunc(args, fallThoughHandler, execHandlers)
			case CallOptionFallThrough:
				execHandlers <- fallThoughHandler
			case CallOptionExit1:
				execHandlers <- exit1
			}
		case scriptErr, ok := <-scriptError:
			if !ok {
				scriptErrDone = true
				scriptError = nil
				continue
			}
			fmt.Println(fmt.Errorf("script failed with error: %w", scriptErr))
		}
	}
}

type CommandOption int

const (
	CallOptionMock = iota
	CallOptionFallThrough
	CallOptionExit1
)

func (o CommandOption) String() string {
	switch o {
	case CallOptionMock:
		return "create shell script to run as mock command"
	case CallOptionFallThrough:
		return "execute command"
	case CallOptionExit1:
		return "simulate failure (exit 1)"
	default:
		panic("unknown command option")
	}
}

func getInterceptionOption(args []string) CommandOption {
	fmt.Printf("# exec intercepted\n%s\n", strings.Join(args, " "))

	options := []CommandOption{
		CallOptionMock,
		CallOptionFallThrough,
		CallOptionExit1,
	}

	var input int
	for {
		fmt.Println("\twhat would you like to do?")
		for i, o := range options {
			fmt.Printf("\t%d: %s\n", i+1, o)
		}
		fmt.Printf("\t> ")
		n, err := fmt.Scanf("%d\n", &input)
		if err != nil || n < 1 || n >= len(options) {
			_, _ = fmt.Fprintf(os.Stderr, "unknown option: %s", err)
			continue
		}
		break
	}
	return options[input-1]
}

func getMockScript(args []string, message string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	tmp, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	_, _ = tmp.WriteString("#!/usr/bin/env bash\n")
	_, _ = tmp.WriteString("set -euo pipefail\n")
	for _, line := range strings.Split(message, "\n") {
		if line == "" {
			continue
		}
		_, _ = tmp.WriteString("### ERROR: " + line + "\n")
	}
	_, _ = tmp.WriteString("### args: ")
	_, _ = tmp.WriteString(strings.Join(args, " "))
	_, _ = tmp.WriteString("\n\n")

	_ = tmp.Close()
	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	contents, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(contents), nil
}

func getAndSendExecFunc(args []string, fallThoughHandler interp.ExecHandlerFunc, c chan<- interp.ExecHandlerFunc) {
	var scriptMessage string
	for {
		mockScriptCode, err := getMockScript(args, scriptMessage)
		if err != nil {
			scriptMessage = err.Error()
			_, _ = fmt.Fprintf(os.Stderr, "failed to get mock script: %s\n", err)
			continue
		}
		r := strings.NewReader(mockScriptCode)
		mockScript, err := syntax.NewParser().Parse(r, "mock.sh")
		if err != nil {
			scriptMessage = err.Error()
			_, _ = fmt.Fprintf(os.Stderr, "failed to parse mock script: %s\n", err)
			continue
		}
		c <- func(ctx context.Context, args []string) error {
			hd := interp.HandlerCtx(ctx)
			runner, err := interp.New(
				interp.StdIO(hd.Stdin, hd.Stdout, hd.Stderr),
				interp.ExecHandler(fallThoughHandler),
				interp.Dir(hd.Dir),
				interp.Env(hd.Env),
				interp.Params(append([]string{"--"}, args[1:]...)...),
			)
			if err != nil {
				panic("failed to start mock shell")
			}
			return runner.Run(context.Background(), mockScript)
		}
		break
	}
}

func exit1(context.Context, []string) error {
	return interp.NewExitStatus(1)
}
