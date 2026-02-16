package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/IAmSurajBobade/go-utils/sctx"
	"github.com/IAmSurajBobade/go-utils/slogger"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

var logger slogger.Logger

func main() {
	inputPath := flag.String("in", "", "Input PDF file or folder path")
	outputFile := flag.String("out", "", "Output PDF file path (optional, default: <input>.pdf)")
	search := flag.String("search", "", "Process only files containing this string (folder mode only)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	logLevel := "info"
	if *verbose {
		logLevel = "debug"
	}

	logger = slogger.NewLoggerWithOptions("aadhar-crop", slogger.Options{
		LevelStr: logLevel,
	})
	ctx := sctx.NewCtx()

	if *inputPath == "" {
		logger.Error(ctx, "Please provide input file or folder using -in flag")
		flag.Usage()
		os.Exit(1)
	}

	fi, err := os.Stat(*inputPath)
	if err != nil {
		logger.Fatal(ctx, "Input path error", "error", err)
	}

	if fi.IsDir() {
		// Process all PDFs in directory
		// Default output dir is <input>/output, but if -out is provided, use that as the directory.
		outDir := *outputFile
		if outDir == "" {
			outDir = filepath.Join(*inputPath, "output")
		}

		if err := os.MkdirAll(outDir, 0755); err != nil {
			logger.Fatal(ctx, "Failed to create output directory", "error", err)
		}

		// Use WalkDir for recursive traversal
		var pdfFiles []string

		err = filepath.WalkDir(*inputPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Skip the output directory itself to avoid infinite loops if it's inside inputPath
			if d.IsDir() && path == outDir {
				return filepath.SkipDir
			}

			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".pdf") {
				// Check search filter
				if *search == "" || strings.Contains(d.Name(), *search) {
					pdfFiles = append(pdfFiles, path)
				}
			}
			return nil
		})

		if err != nil {
			logger.Fatal(ctx, "Failed to walk directory", "error", err)
		}

		if len(pdfFiles) == 0 {
			msg := fmt.Sprintf("No PDF files found in %s", *inputPath)
			if *search != "" {
				msg += fmt.Sprintf(" matching '%s'", *search)
			}
			logger.Info(ctx, msg)
			return
		}

		logger.Info(ctx, fmt.Sprintf("Processing %d PDF files...", len(pdfFiles)), "count", len(pdfFiles))

		successCount := 0
		errorCount := 0

		for _, inPath := range pdfFiles {
			// Default output: filename.pdf.
			fName := filepath.Base(inPath)
			ext := filepath.Ext(fName)
			baseName := strings.TrimSuffix(fName, ext)
			outName := baseName + ".pdf"
			out := filepath.Join(outDir, outName)

			if err := processFile(ctx, inPath, out); err != nil {
				logger.Error(ctx, "Error processing file", "file", fName, "error", err)
				errorCount++
			} else {
				successCount++
			}
		}
		logger.Info(ctx, "Processing complete", "output_dir", outDir, "processed", successCount, "errors", errorCount)
	} else {
		// Process single file
		outPath := *outputFile
		if outPath == "" {
			dir := filepath.Dir(*inputPath)
			ext := filepath.Ext(*inputPath)
			baseName := strings.TrimSuffix(filepath.Base(*inputPath), ext)
			outPath = filepath.Join(dir, baseName+".pdf")
		}

		if err := processFile(ctx, *inputPath, outPath); err != nil {
			logger.Fatal(ctx, "Processing failed", "error", err)
		}
		logger.Info(ctx, "Successfully processed file", "output", outPath)
	}
}

// processFile handles the crop and merge for a single PDF
func processFile(ctx context.Context, inputFile, outputFile string) error {
	// Create a temporary directory for intermediate files
	tmpDir, err := os.MkdirTemp("", "aadhar-crop-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Crop region definitions
	frontBox := &model.Box{Rect: types.NewRectangle(20, 20, 303, 220)}
	backBox := &model.Box{Rect: types.NewRectangle(308, 20, 590, 220)}

	frontWidth := 303.0 - 20.0
	frontHeight := 220.0 - 20.0
	backWidth := 590.0 - 308.0
	cardWidth := max(frontWidth, backWidth)
	cardHeight := frontHeight

	logger.Debug(ctx, "Card dimensions calculated", "file", filepath.Base(inputFile), "width", cardWidth, "height", cardHeight)

	frontTmp := filepath.Join(tmpDir, "front.pdf")
	backTmp := filepath.Join(tmpDir, "back.pdf")

	conf := model.NewDefaultConfiguration() // For watermarking
	cropConf := model.NewDefaultConfiguration()
	cropConf.Cmd = model.CROP

	// Crop
	logger.Debug(ctx, "Cropping front side...", "file", filepath.Base(inputFile))
	if err := api.CropFile(inputFile, frontTmp, []string{"1"}, frontBox, cropConf); err != nil {
		return fmt.Errorf("crop front failed: %w", err)
	}
	logger.Debug(ctx, "Cropping back side...", "file", filepath.Base(inputFile))
	if err := api.CropFile(inputFile, backTmp, []string{"1"}, backBox, cropConf); err != nil {
		return fmt.Errorf("crop back failed: %w", err)
	}

	// Layout (Centered on A4)
	pageWidth, pageHeight := 595.0, 842.0
	gap := 20.0
	totalHeight := cardHeight + gap + cardHeight
	centerX := (pageWidth - cardWidth) / 2.0
	topMargin := (pageHeight - totalHeight) / 3.0

	frontOffX, frontOffY := centerX-20, -topMargin
	backOffX, backOffY := centerX+10, -(topMargin + cardHeight + gap)

	logger.Debug(ctx, "Layout calculated", "file", filepath.Base(inputFile),
		"page_width", pageWidth, "page_height", pageHeight,
		"front_off_x", frontOffX, "front_off_y", frontOffY,
		"back_off_x", backOffX, "back_off_y", backOffY)

	// Create Canvas
	logger.Debug(ctx, "Creating blank canvas...", "file", filepath.Base(inputFile))
	canvasTmp := filepath.Join(tmpDir, "canvas.pdf")
	if err := createBlankPDF(canvasTmp); err != nil {
		return fmt.Errorf("failed to create blank pdf: %w", err)
	}

	// Stamp
	logger.Debug(ctx, "Stamping front section...", "file", filepath.Base(inputFile))
	frontDesc := fmt.Sprintf("pos:tl, off:%.0f %.0f, rot:0, scale:1 abs", frontOffX, frontOffY)
	wmConfFront, err := api.PDFWatermark(frontTmp, frontDesc, true, false, types.POINTS)
	if err != nil {
		return fmt.Errorf("failed to create front watermark: %w", err)
	}
	step1Tmp := filepath.Join(tmpDir, "step1.pdf")
	if err := api.AddWatermarksFile(canvasTmp, step1Tmp, []string{"1"}, wmConfFront, conf); err != nil {
		return fmt.Errorf("stamp front failed: %w", err)
	}

	logger.Debug(ctx, "Stamping back section...", "file", filepath.Base(inputFile))
	backDesc := fmt.Sprintf("pos:tl, off:%.0f %.0f, rot:0, scale:1 abs", backOffX, backOffY)
	wmConfBack, err := api.PDFWatermark(backTmp, backDesc, true, false, types.POINTS)
	if err != nil {
		return fmt.Errorf("failed to create back watermark: %w", err)
	}
	if err := api.AddWatermarksFile(step1Tmp, outputFile, []string{"1"}, wmConfBack, conf); err != nil {
		return fmt.Errorf("stamp back failed: %w", err)
	}

	// Optimize
	if err := api.OptimizeFile(outputFile, outputFile, model.NewDefaultConfiguration()); err != nil {
		logger.Warn(ctx, "Optimization failed", "file", filepath.Base(inputFile), "error", err)
	}

	// Report file sizes
	inInfo, _ := os.Stat(inputFile)
	outInfo, _ := os.Stat(outputFile)
	logger.Debug(ctx, "File sizes", "file", filepath.Base(inputFile),
		"input_kb", float64(inInfo.Size())/1024,
		"output_kb", float64(outInfo.Size())/1024)

	logger.Debug(ctx, "Successfully created output file", "file", outputFile)
	return nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// createBlankPDF creates a minimal, valid PDF 1.7 file with a single blank A4 page.
// This avoids using image imports which bloat file size.
func createBlankPDF(outFile string) error {
	// Minimal PDF structure
	// This defines a PDF with 1 page of size 595x842 (A4).
	content := `%PDF-1.7
1 0 obj
<</Type /Catalog /Pages 2 0 R>>
endobj
2 0 obj
<</Type /Pages /Kids [3 0 R] /Count 1>>
endobj
3 0 obj
<</Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources <<>>>>
endobj
xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000050 00000 n
0000000097 00000 n
trailer
<</Root 1 0 R /Size 4>>
startxref
174
%%EOF`

	return os.WriteFile(outFile, []byte(content), 0644)
}
