package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
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
				Usage:   "enable the debug mode to show more debug information",
				EnvVars: []string{"DEBUG_MODE"},
			},
			&cli.BoolFlag{
				Name:    "trace",
				Aliases: []string{"t"},
				Usage:   "enable the trace mode to show more and more debug information",
				EnvVars: []string{"TRACE_MODE"},
			},
			&cli.StringFlag{
				Name:    "folder",
				Aliases: []string{"f"},
				Usage:   "the data folder of the input list and the output vcf, png files",
				Value:   "testdata",
			},
			&cli.StringFlag{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "list of all contacts",
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

type contact struct {
	Class    string
	Fullname string
	VcfFname string
	Cell     string
	Email    string
}

func mainAction(c *cli.Context) error {
	logger := log.Ctx(c.Context)

	dataFolder := c.String("folder")
	lstFname := c.String("list")

	if lstFname != "" {
		contactLst, err := parseInputList(c.Context, dataFolder, lstFname)
		if err != nil {
			return err
		}
		logger.Info().Msgf("read %d contacts", len(contactLst))

		for idx, cnt := range contactLst {
			logger.Debug().Interface("c", cnt).Msgf("%d", idx+1)
			if _, err := os.Stat(cnt.VcfFname); errors.Is(err, os.ErrNotExist) {
				// vCard file is not exist
				if err := generateVCard(c.Context, cnt); err != nil {
					return err
				}
			}
			if err := geneateQRCodeByFile(cnt.VcfFname); err != nil {
				return err
			}
		}

	}

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

func parseInputList(ctx context.Context, dataFolder, lstFname string) ([]*contact, error) {
	logger := log.Ctx(ctx).With().Str("folder", dataFolder).Str("lst", lstFname).Logger()
	fin, err := os.Open(filepath.Join(dataFolder, lstFname))
	if err != nil {
		return nil, err
	}
	defer fin.Close()

	r := csv.NewReader(fin)
	contactLst := make([]*contact, 0)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if v, err := strconv.ParseInt(rec[0], 10, 64); err != nil || v == 0 {
			if err != nil {
				logger.Debug().Err(err).Msgf("error to parse the row: %s", rec)
			}
			continue
		}

		oneContact := new(contact)
		oneContact.Class = rec[1]
		oneContact.Fullname = rec[2]
		oneContact.VcfFname = filepath.Join(dataFolder, rec[3]+defaultVCFExtension)
		oneContact.Cell = formatCellNo(rec[4])
		oneContact.Email = rec[5]

		contactLst = append(contactLst, oneContact)
	}

	return contactLst, nil
}

func formatCellNo(orig string) string {
	if len(orig) != 10 {
		return orig
	}
	if !strings.HasPrefix(orig, "09") {
		return orig
	}

	return fmt.Sprintf("+886 %s-%s-%s", orig[1:4], orig[4:7], orig[7:])
}

func generateVCard(ctx context.Context, cnt *contact) error {
	logger := log.Ctx(ctx).With().Str("fn", cnt.Fullname).Logger()
	var sb strings.Builder

	// begin of vCard
	sb.WriteString("BEGIN:VCARD\nVERSION:3.0\n")

	// FN
	sb.WriteString(fmt.Sprintf("FN:%s\n", cnt.Fullname))
	// N
	nRunes := []rune(cnt.Fullname)
	sb.WriteString(fmt.Sprintf("N:%s;%s;;;\n", string(nRunes[0]), string(nRunes[1:])))
	// Email
	if cnt.Email != "" {
		sb.WriteString(fmt.Sprintf("EMAIL;TYPE=INTERNET;TYPE=WORK:%s\n", cnt.Email))
	}
	// TEL/CELL
	if cnt.Cell != "" {
		sb.WriteString(fmt.Sprintf("TEL;TYPE=CELL:%s\n", cnt.Cell))
	}
	// NOTE
	sb.WriteString(fmt.Sprintf("NOTE:建中42屆%s班同學\n", cnt.Class))

	// end of vCard
	sb.WriteString("END:VCARD\n")

	if err := os.WriteFile(cnt.VcfFname, []byte(sb.String()), 0666); err != nil {
		return err
	}

	logger.Trace().Msg("vCard generated")
	return nil
}
