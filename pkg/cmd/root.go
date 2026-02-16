package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/IAmSurajBobade/go-utils/sctx"
	"github.com/IAmSurajBobade/go-utils/slogger"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	Logger  slogger.Logger
	Ctx     context.Context
)

var rootCmd = &cobra.Command{
	Use:   "utils",
	Short: "A collection of utility scripts",
	Long: `A CLI tool for various utility scripts.

Common actions include cropping Aadhar PDFs using the 'aadhar' subcommand.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logLevel := "info"
		if verbose {
			logLevel = "debug"
		}

		Logger = slogger.NewLoggerWithOptions("utils", slogger.Options{
			LevelStr: logLevel,
		})
		Ctx = sctx.NewCtx()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	// Adjust help examples based on OS
	example := ""
	if runtime.GOOS == "windows" {
		example = "  utils aadhar --in C:\\path\\to\\input --out C:\\path\\to\\output"
	} else {
		example = "  utils aadhar --in /path/to/input --out /path/to/output"
	}
	rootCmd.Example = example
}
