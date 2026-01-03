package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func main() {
	inputPath := flag.String("in", "", "Input PDF file or folder path")
	outputFile := flag.String("out", "output.pdf", "Output PDF file path (ignored for folder input)")
	flag.Parse()

	if *inputPath == "" {
		fmt.Println("Please provide input file or folder using -in flag")
		flag.Usage()
		os.Exit(1)
	}

	fi, err := os.Stat(*inputPath)
	if err != nil {
		log.Fatalf("Input path error: %v", err)
	}

	if fi.IsDir() {
		// Process all PDFs in directory
		outDir := filepath.Join(*inputPath, "output")
		if err := os.MkdirAll(outDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}

		entries, err := os.ReadDir(*inputPath)
		if err != nil {
			log.Fatalf("Failed to read directory: %v", err)
		}

		var pdfFiles []string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".pdf" {
				pdfFiles = append(pdfFiles, entry.Name())
			}
		}

		if len(pdfFiles) == 0 {
			fmt.Printf("No PDF files found in %s\n", *inputPath)
			return
		}

		fmt.Printf("Processing %d PDF files...\n", len(pdfFiles))
		for _, f := range pdfFiles {
			in := filepath.Join(*inputPath, f)
			out := filepath.Join(outDir, "cropped_"+f)
			if err := processFile(in, out); err != nil {
				log.Printf("Error processing %s: %v", f, err)
			}
		}
		fmt.Printf("Done! Outputs saved in: %s\n", outDir)
	} else {
		// Process single file
		if err := processFile(*inputPath, *outputFile); err != nil {
			log.Fatalf("Processing failed: %v", err)
		}
	}
}

// processFile handles the crop and merge for a single PDF
func processFile(inputFile, outputFile string) error {
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

	log.Printf("[%s] Card dimensions: %.0f x %.0f", filepath.Base(inputFile), cardWidth, cardHeight)

	frontTmp := filepath.Join(tmpDir, "front.pdf")
	backTmp := filepath.Join(tmpDir, "back.pdf")

	conf := model.NewDefaultConfiguration() // For watermarking
	cropConf := model.NewDefaultConfiguration()
	cropConf.Cmd = model.CROP

	// Crop
	log.Printf("[%s] Cropping front side...", filepath.Base(inputFile))
	if err := api.CropFile(inputFile, frontTmp, []string{"1"}, frontBox, cropConf); err != nil {
		return fmt.Errorf("crop front failed: %w", err)
	}
	log.Printf("[%s] Cropping back side...", filepath.Base(inputFile))
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

	log.Printf("[%s] Page size: %.0f x %.0f", filepath.Base(inputFile), pageWidth, pageHeight)
	log.Printf("[%s] Front position: off:%.0f %.0f", filepath.Base(inputFile), frontOffX, frontOffY)
	log.Printf("[%s] Back position: off:%.0f %.0f", filepath.Base(inputFile), backOffX, backOffY)

	// Create Canvas
	log.Printf("[%s] Creating blank canvas...", filepath.Base(inputFile))
	canvasTmp := filepath.Join(tmpDir, "canvas.pdf")
	createBlankPDF(canvasTmp, int(pageWidth), int(pageHeight))

	// Stamp
	log.Printf("[%s] Stamping front section...", filepath.Base(inputFile))
	frontDesc := fmt.Sprintf("pos:tl, off:%.0f %.0f, rot:0, scale:1 abs", frontOffX, frontOffY)
	log.Printf("[%s] Front watermark: %s", filepath.Base(inputFile), frontDesc)
	wmConfFront, err := api.PDFWatermark(frontTmp, frontDesc, true, false, types.POINTS)
	if err != nil {
		return fmt.Errorf("failed to create front watermark: %w", err)
	}
	step1Tmp := filepath.Join(tmpDir, "step1.pdf")
	if err := api.AddWatermarksFile(canvasTmp, step1Tmp, []string{"1"}, wmConfFront, conf); err != nil {
		return fmt.Errorf("stamp front failed: %w", err)
	}

	log.Printf("[%s] Stamping back section...", filepath.Base(inputFile))
	backDesc := fmt.Sprintf("pos:tl, off:%.0f %.0f, rot:0, scale:1 abs", backOffX, backOffY)
	log.Printf("[%s] Back watermark: %s", filepath.Base(inputFile), backDesc)
	wmConfBack, err := api.PDFWatermark(backTmp, backDesc, true, false, types.POINTS)
	if err != nil {
		return fmt.Errorf("failed to create back watermark: %w", err)
	}
	if err := api.AddWatermarksFile(step1Tmp, outputFile, []string{"1"}, wmConfBack, conf); err != nil {
		return fmt.Errorf("stamp back failed: %w", err)
	}

	// Optimize
	if err := api.OptimizeFile(outputFile, outputFile, model.NewDefaultConfiguration()); err != nil {
		log.Printf("[%s] Warning: optimization failed for %s: %v", filepath.Base(inputFile), outputFile, err)
	}

	// Report file sizes
	inInfo, _ := os.Stat(inputFile)
	outInfo, _ := os.Stat(outputFile)
	log.Printf("[%s] Input size: %.1f KB", filepath.Base(inputFile), float64(inInfo.Size())/1024)
	log.Printf("[%s] Output size: %.1f KB", filepath.Base(inputFile), float64(outInfo.Size())/1024)

	log.Printf("Successfully created %s", outputFile)
	return nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// createBlankPDF creates a single page PDF with white background
func createBlankPDF(outFile string, width, height int) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with white
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, white)
		}
	}

	f, err := os.CreateTemp("", "blank-*.png")
	if err != nil {
		log.Fatalf("Failed to create temp png: %v", err)
	}
	tmpName := f.Name()
	defer os.Remove(tmpName)

	if err := png.Encode(f, img); err != nil {
		log.Fatalf("Failed to encode png: %v", err)
	}
	f.Close()

	// Import with exact dimensions
	imp := pdfcpu.DefaultImportConfig()
	imp.PageDim = &types.Dim{Width: float64(width), Height: float64(height)}

	if err := api.ImportImagesFile([]string{tmpName}, outFile, imp, model.NewDefaultConfiguration()); err != nil {
		log.Fatalf("Failed to create blank pdf: %v", err)
	}
}
