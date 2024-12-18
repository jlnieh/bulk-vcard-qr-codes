package main

import (
	"context"
	"encoding/csv"
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
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
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
				Usage:   "list of all contacts; which is exported from Excel with TAB delimiter and UTF-16 LE encoding with BOM",
			},
			&cli.StringFlag{
				Name:    "excel",
				Aliases: []string{"e"},
				Usage:   "output excel files to include the class, name and QR code image; this option must be used with input 'list' file",
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

type answerType int

const (
	AnswerNo     answerType = 0
	AnswerYes    answerType = 1
	AnswerCustom answerType = 2
	AnswerCancel answerType = -1
)

type contact struct {
	Class    string
	Fullname string
	VcfFname string
	Cell     string
	Email    string
	Answer   answerType
}

func mainAction(c *cli.Context) error {
	logger := log.Ctx(c.Context)

	if c.NArg() > 0 {
		for _, vcfFname := range c.Args().Slice() {
			if err := geneateQRCodeByFile(vcfFname); err != nil {
				return err
			}
		}
	}

	dataFolder := c.String("folder")
	lstFname := c.String("list")
	outExcelFname := c.String("excel")

	if lstFname == "" {
		return nil
	}

	// followings are work with the input list file
	contactLst, err := parseInputList(c.Context, dataFolder, lstFname)
	if err != nil {
		return err
	}
	logger.Info().Msgf("read %d contacts", len(contactLst))

	for idx, cnt := range contactLst {
		logger.Debug().Interface("c", cnt).Msgf("%d", idx+1)

		if cnt.Answer == AnswerCustom {
			if _, err := os.Stat(cnt.VcfFname); err != nil { // errors.Is(err, os.ErrNotExist)
				logger.Error().Err(err).Interface("cnt", cnt).Msg("the record required customized vCard, which has error")
				return err
			}
		} else if err := generateVCard(c.Context, cnt); err != nil { // vCard file is not exist
			return err
		}

		if err := geneateQRCodeByFile(cnt.VcfFname); err != nil {
			return err
		}
	}

	if outExcelFname != "" {
		outExcelFname = filepath.Join(dataFolder, outExcelFname)
		if err := generateExcelFile(c.Context, outExcelFname, contactLst); err != nil {
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

	// Create a UTF-16 decoder
	decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()

	r := csv.NewReader(transform.NewReader(fin, decoder))
	r.Comma = '\t' // Set the delimiter to tab

	contactLst := make([]*contact, 0)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if rec[0] == "no" {
			continue // the first line
		}

		if v, err := strconv.ParseInt(rec[0], 10, 64); err != nil || v == 0 {
			if err != nil {
				logger.Error().Err(err).Msgf("error to parse the row: %s", rec)
			}
			continue
		}

		oneContact := new(contact)
		oneContact.Class = rec[1]
		oneContact.Fullname = rec[2]
		oneContact.VcfFname = filepath.Join(dataFolder, rec[3]+defaultVCFExtension)
		oneContact.Cell = formatCellNo(rec[4])
		oneContact.Email = rec[5]
		if v, err := strconv.ParseInt(rec[6], 10, 64); err != nil || v < 0 {
			if err != nil {
				logger.Error().Err(err).Msgf("error to parse the answer of the row: %s", rec)
			} else {
				logger.Debug().Msgf("skip the cancelled record: %s", rec)
			}
			continue
		} else {
			oneContact.Answer = answerType(v)
		}

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

	if cnt.Answer == AnswerYes {
		// Email
		if cnt.Email != "" {
			sb.WriteString(fmt.Sprintf("EMAIL;TYPE=INTERNET;TYPE=WORK:%s\n", cnt.Email))
		}
		// TEL/CELL
		if cnt.Cell != "" {
			sb.WriteString(fmt.Sprintf("TEL;TYPE=CELL:%s\n", cnt.Cell))
		}
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

func generateExcelFile(ctx context.Context, outFname string, contactList []*contact) error {
	logger := log.Ctx(ctx).With().Str("out", outFname).Logger()
	fout := excelize.NewFile()
	defer func() {
		if err := fout.Close(); err != nil {
			logger.Error().Err(err).Msg("error to generate the excel file")
		}
	}()

	if err := fillListSheet(ctx, fout, contactList); err != nil {
		return err
	}
	if err := genQRCodeSheet(ctx, fout, contactList); err != nil {
		return err
	}

	// Save spreadsheet by the given path.
	if err := fout.SaveAs(outFname); err != nil {
		return err
	}

	logger.Info().Msg("Excel output done!")
	return nil
}

func fillListSheet(ctx context.Context, fout *excelize.File, contactList []*contact) error {
	const sheet1Name = "Sheet1"

	logger := log.Ctx(ctx)

	// set header
	if err := fout.SetCellValue(sheet1Name, "A1", "class"); err != nil {
		return err
	}
	if err := fout.SetCellValue(sheet1Name, "B1", "name"); err != nil {
		return err
	}

	for idx, cnt := range contactList {
		rowID := strconv.FormatInt(int64(idx+2), 10)
		if err := fout.SetCellValue(sheet1Name, "A"+rowID, cnt.Class); err != nil {
			logger.Error().Err(err).Msg("error to set contact class")
			break
		}
		if err := fout.SetCellValue(sheet1Name, "B"+rowID, cnt.Fullname); err != nil {
			logger.Error().Err(err).Msg("error to set contact name")
			break
		}
	}

	return nil
}

func genQRCodeSheet(ctx context.Context, fout *excelize.File, contactList []*contact) error {
	const sheet2Name = "Sheet2"

	logger := log.Ctx(ctx)
	// Create a new sheet.
	if _, err := fout.NewSheet(sheet2Name); err != nil {
		return err
	}

	// a4Size := int(9)
	// fout.SetPageLayout(sheetName, &excelize.PageLayoutOptions{Size: &a4Size})
	mB := float64(0.04)
	mF := float64(0.3)
	mL := float64(0.7)
	vhCenter := true
	if err := fout.SetPageMargins(sheet2Name, &excelize.PageLayoutMarginsOptions{
		Bottom:       &mB,
		Footer:       &mF,
		Header:       &mF,
		Top:          &mB,
		Left:         &mL,
		Right:        &mL,
		Horizontally: &vhCenter,
		Vertically:   &vhCenter,
	}); err != nil {
		return err
	}

	// set column width, style...
	fout.SetColWidth(sheet2Name, "A", "B", 36)
	txtStyleID, err := fout.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "top", Style: 2, Color: "000000"},
			{Type: "left", Style: 2, Color: "000000"},
			{Type: "right", Style: 2, Color: "000000"},
		},
	})
	if err != nil {
		return err
	}
	imgStyleID, err := fout.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
		Border: []excelize.Border{
			{Type: "left", Style: 2, Color: "000000"},
			{Type: "right", Style: 2, Color: "000000"},
			{Type: "bottom", Style: 2, Color: "000000"},
		},
	})
	if err != nil {
		return err
	}

	enabled := true
	defGraphicOpts := &excelize.GraphicOptions{
		PrintObject:     &enabled,
		LockAspectRatio: true,
		// AutoFit:         true,
		OffsetX:     16,
		OffsetY:     1,
		ScaleX:      0.78,
		ScaleY:      0.711,
		Positioning: "oneCell",
	}
	for idx, cnt := range contactList {
		rowID := int(idx/2)*2 + 1
		var colID = "A"
		if (idx % 2) == 0 {
			// set row height
			// fout.SetRowHeight(sheetName, rowID, 15) // use standard
			fout.SetRowHeight(sheet2Name, rowID+1, 155)
		} else {
			colID = "B"
		}
		txtCellID := colID + strconv.FormatInt(int64(rowID), 10)
		imgCellID := colID + strconv.FormatInt(int64(rowID+1), 10)

		cellText := cnt.Class + " " + cnt.Fullname
		imgFname := strings.ReplaceAll(cnt.VcfFname, defaultVCFExtension, ".png")

		fout.SetCellStyle(sheet2Name, txtCellID, txtCellID, txtStyleID)
		if err := fout.SetCellValue(sheet2Name, txtCellID, cellText); err != nil {
			logger.Error().Err(err).Msg("error to set QR title")
			break
		}

		fout.SetCellStyle(sheet2Name, imgCellID, imgCellID, imgStyleID)
		if err := fout.AddPicture(sheet2Name, imgCellID, imgFname, defGraphicOpts); err != nil {
			logger.Error().Err(err).Msg("error to add QR code")
			break
		}
	}
	return nil
}
