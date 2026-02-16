package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputFile string
	search     string
)

var aadharCmd = &cobra.Command{
	Use:   "aadhar",
	Short: "Crop and watermark Aadhar PDF files",
	Long: `Crops Aadhar PDF files to extract the card section and watermarks it.
Supports single file or bulk folder processing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if inputPath == "" {
			return fmt.Errorf("please provide input file or folder using --in flag")
		}

		fi, err := os.Stat(inputPath)
		if err != nil {
			return fmt.Errorf("input path error: %w", err)
		}

		if fi.IsDir() {
			return processDirectory(inputPath, outputFile, search)
		}

		return processSingleFile(inputPath, outputFile)
	},
}

func init() {
	rootCmd.AddCommand(aadharCmd)

	aadharCmd.Flags().StringVar(&inputPath, "in", "", "Input PDF file or folder path")
	aadharCmd.Flags().StringVar(&outputFile, "out", "", "Output PDF file path (optional, default: <input>.pdf)")
	aadharCmd.Flags().StringVar(&search, "search", "", "Process only files containing this string (folder mode only)")

	// Mark 'in' as required ideally, but we handle error manually as per original logic request
	// aadharCmd.MarkFlagRequired("in")
}

func processDirectory(inPath, outPath, searchStr string) error {
	// Default output dir is <input>/output, but if -out is provided, use that as the directory.
	outDir := outPath
	if outDir == "" {
		outDir = filepath.Join(inPath, "output")
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		Logger.Fatal(Ctx, "Failed to create output directory", "error", err)
	}

	// Use WalkDir for recursive traversal
	var pdfFiles []string

	err := filepath.WalkDir(inPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the output directory itself to avoid infinite loops if it's inside inputPath
		if d.IsDir() && path == outDir {
			return filepath.SkipDir
		}

		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".pdf") {
			// Check search filter
			if searchStr == "" || strings.Contains(d.Name(), searchStr) {
				pdfFiles = append(pdfFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		Logger.Fatal(Ctx, "Failed to walk directory", "error", err)
	}

	if len(pdfFiles) == 0 {
		msg := fmt.Sprintf("No PDF files found in %s", inPath)
		if searchStr != "" {
			msg += fmt.Sprintf(" matching '%s'", searchStr)
		}
		Logger.Info(Ctx, msg)
		return nil
	}

	Logger.Info(Ctx, fmt.Sprintf("Processing %d PDF files...", len(pdfFiles)), "count", len(pdfFiles))

	successCount := 0
	errorCount := 0

	for _, p := range pdfFiles {
		// Default output: filename.pdf.
		fName := filepath.Base(p)
		ext := filepath.Ext(fName)
		baseName := strings.TrimSuffix(fName, ext)
		outName := baseName + ".pdf"
		out := filepath.Join(outDir, outName)

		if err := processFile(Ctx, p, out); err != nil {
			Logger.Error(Ctx, "Error processing file", "file", fName, "error", err)
			errorCount++
		} else {
			successCount++
		}
	}
	Logger.Info(Ctx, "Processing complete", "output_dir", outDir, "processed", successCount, "errors", errorCount)
	return nil
}

func processSingleFile(inPath, outPath string) error {
	finalOutPath := outPath
	if finalOutPath == "" {
		dir := filepath.Dir(inPath)
		ext := filepath.Ext(inPath)
		baseName := strings.TrimSuffix(filepath.Base(inPath), ext)
		finalOutPath = filepath.Join(dir, baseName+".pdf")
	}

	if err := processFile(Ctx, inPath, finalOutPath); err != nil {
		Logger.Fatal(Ctx, "Processing failed", "error", err)
	}
	Logger.Info(Ctx, "Successfully processed file", "output", finalOutPath)
	return nil
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

	Logger.Debug(ctx, "Card dimensions calculated", "file", filepath.Base(inputFile), "width", cardWidth, "height", cardHeight)

	frontTmp := filepath.Join(tmpDir, "front.pdf")
	backTmp := filepath.Join(tmpDir, "back.pdf")

	conf := model.NewDefaultConfiguration() // For watermarking
	cropConf := model.NewDefaultConfiguration()
	cropConf.Cmd = model.CROP

	// Crop
	Logger.Debug(ctx, "Cropping front side...", "file", filepath.Base(inputFile))
	if err := api.CropFile(inputFile, frontTmp, []string{"1"}, frontBox, cropConf); err != nil {
		return fmt.Errorf("crop front failed: %w", err)
	}
	Logger.Debug(ctx, "Cropping back side...", "file", filepath.Base(inputFile))
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

	Logger.Debug(ctx, "Layout calculated", "file", filepath.Base(inputFile),
		"page_width", pageWidth, "page_height", pageHeight,
		"front_off_x", frontOffX, "front_off_y", frontOffY,
		"back_off_x", backOffX, "back_off_y", backOffY)

	// Create Canvas
	Logger.Debug(ctx, "Creating blank canvas...", "file", filepath.Base(inputFile))
	canvasTmp := filepath.Join(tmpDir, "canvas.pdf")
	if err := createBlankPDF(canvasTmp); err != nil {
		return fmt.Errorf("failed to create blank pdf: %w", err)
	}

	// Stamp
	Logger.Debug(ctx, "Stamping front section...", "file", filepath.Base(inputFile))
	frontDesc := fmt.Sprintf("pos:tl, off:%.0f %.0f, rot:0, scale:1 abs", frontOffX, frontOffY)
	wmConfFront, err := api.PDFWatermark(frontTmp, frontDesc, true, false, types.POINTS)
	if err != nil {
		return fmt.Errorf("failed to create front watermark: %w", err)
	}
	step1Tmp := filepath.Join(tmpDir, "step1.pdf")
	if err := api.AddWatermarksFile(canvasTmp, step1Tmp, []string{"1"}, wmConfFront, conf); err != nil {
		return fmt.Errorf("stamp front failed: %w", err)
	}

	Logger.Debug(ctx, "Stamping back section...", "file", filepath.Base(inputFile))
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
		Logger.Warn(ctx, "Optimization failed", "file", filepath.Base(inputFile), "error", err)
	}

	// Report file sizes
	inInfo, _ := os.Stat(inputFile)
	outInfo, _ := os.Stat(outputFile)
	Logger.Debug(ctx, "File sizes", "file", filepath.Base(inputFile),
		"input_kb", float64(inInfo.Size())/1024,
		"output_kb", float64(outInfo.Size())/1024)

	Logger.Debug(ctx, "Successfully created output file", "file", outputFile)
	return nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// createBlankPDF creates a minimal, valid PDF 1.7 file with a single blank A4 page.
func createBlankPDF(outFile string) error {
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
