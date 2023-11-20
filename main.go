package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

func main() {
	if err := _main(); err != nil {
		if str := err.Error(); !strings.HasPrefix("exit status ", str) {
			fmt.Println(str)
		}
		var eithExitCode *ErrorWithExitCode
		if errors.As(err, &eithExitCode) {
			os.Exit(eithExitCode.ExitCode)
		} else {
			os.Exit(1)
		}
	}
}

func _main() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var cli CLI
	err := cli.Parse(os.Args[1:])
	if err != nil {
		return err
	}
	app, err := New(cli)
	if err != nil {
		return err
	}
	if err := app.Run(ctx); err != nil {
		return err
	}
	return nil
}
