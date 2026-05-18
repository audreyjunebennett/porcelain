package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/supervise"
	"github.com/lynn/porcelain/internal/naming"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" {
			config.PrintHelp()
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", naming.ProductSupervisorName, err)
		os.Exit(exitCodeForError(err))
	}
}

func exitCodeForError(err error) int {
	var ee *config.ExitError
	if errors.As(err, &ee) && ee.Code != 0 {
		return ee.Code
	}
	return 1
}

func run(args []string) error {
	cfg, err := config.Parse(args, config.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return supervise.Run(context.Background(), cfg, version, commit)
}
