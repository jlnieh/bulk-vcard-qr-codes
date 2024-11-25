package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/urfave/cli/v2"
)

const cmdToolVersion = "v0.0.1"

var (
	commitID  = "0000000"
	buildTime = "2020-01-03T14:32:00+08:00"
)

func main() {
	var execBasename string

	// Setup the default logger to ConsoleWriter
	zerolog.DisableSampling(true)
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.TimeOnly,
	})

	// get executable name
	if fn, err := os.Executable(); err != nil {
		logger.Panic().Err(err).Msg("error to get executable name")
		return
	} else {
		execBasename = filepath.Base(fn)
	}

	// setup cli app
	app := &cli.App{
		Version:         fmt.Sprintf("%s build %s at %s", cmdToolVersion, commitID, buildTime),
		Usage:           "tool to generate vCard QR codes by a list of contacts",
		UsageText:       fmt.Sprintf("%s [options] [contact.vcf]", execBasename),
		HideHelpCommand: false,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "enable the debug mode to use the latest Lambda function in Cloud and to show more debug information",
				EnvVars: []string{"DEBUG_ADMINTOOL"},
			},
			&cli.BoolFlag{
				Name:    "trace",
				Aliases: []string{"t"},
				Usage:   "enable the trace mode to show more and more debug information",
				EnvVars: []string{"DEBUG_ADMINTOOL"},
			},
		},
		Before: func(c *cli.Context) error {
			if c.Bool("debug") {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			} else if c.Bool("trace") {
				zerolog.SetGlobalLevel(zerolog.TraceLevel)
			}
			return nil
		},
		Action: mainAction,
	}

	ctx := logger.WithContext(context.Background())
	if err := app.RunContext(ctx, os.Args); err != nil {
		logger.Fatal().Err(err).Msgf("failed to execute %s", execBasename)
	}
}

const (
	defaultVCFExtension = ".vcf"
)

func mainAction(c *cli.Context) error {
	if c.NArg() == 0 {
		return nil
	}

	for _, vcfFname := range c.Args().Slice() {
		if err := geneateQRCodeByFile(vcfFname); err != nil {
			return err
		}
	}

	return nil
}

func geneateQRCodeByFile(vcfFname string) error {
	var outFname string
	if fname := strings.Trim(vcfFname, defaultVCFExtension); fname != "" {
		outFname = fmt.Sprintf("%s.png", fname)
	} else {
		return fmt.Errorf("invalid vCard file name: %s", vcfFname)
	}

	if content, err := os.ReadFile(vcfFname); err != nil {
		return err
	} else if len(content) == 0 {
		return fmt.Errorf("empty vCard file: %s", vcfFname)
	} else {
		qrcode.WriteFile(string(content), qrcode.Medium, 256, outFname)
	}

	return nil
}
