package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/specguard/specguard/internal/llm"
	"github.com/jung-kurt/gofpdf"
)

func generatePDFReport(pdfPath, projectName, reportsDir string, sections llm.SectionSummaries) error {
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		return fmt.Errorf("create pdf dir: %w", err)
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)

	// Title page
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.Ln(40)
	pdf.CellFormat(0, 15, "SpecGuard", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 18)
	pdf.CellFormat(0, 12, "API Governance Report", "", 1, "C", false, 0, "")
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "", 14)
	if projectName != "" {
		pdf.CellFormat(0, 10, "Project: "+sanitize(projectName), "", 1, "C", false, 0, "")
	}
	pdf.CellFormat(0, 10, "Generated: "+time.Now().Format("January 2, 2006 15:04 MST"), "", 1, "C", false, 0, "")
	pdf.Ln(20)

	// Executive Summary
	if sections.Executive != "" {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetFillColor(240, 240, 240)
		pdf.CellFormat(0, 8, "Executive Summary", "", 1, "L", true, 0, "")
		pdf.Ln(3)
		pdf.SetFont("Helvetica", "", 10)
		writeWrapped(pdf, sections.Executive)
		pdf.Ln(6)
	}

	// Table of Contents
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Contents", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	tocItems := []string{
		"1. Risk Assessment",
		"2. Standards Compliance",
		"3. Documentation Consistency",
		"4. Protocol Recommendations",
		"5. Documentation Enrichment",
	}
	for _, item := range tocItems {
		pdf.CellFormat(0, 6, "    "+item, "", 1, "L", false, 0, "")
	}
	pdf.Ln(6)

	// Section 1: Risk
	pdf.AddPage()
	writeSectionHeader(pdf, "1. Risk Assessment")
	if sections.Risk != "" {
		writeSummaryBox(pdf, sections.Risk)
	}
	writeMDSection(pdf, filepath.Join(reportsDir, "risk.md"))

	// Section 2: Standards
	pdf.AddPage()
	writeSectionHeader(pdf, "2. Standards Compliance")
	if sections.Standards != "" {
		writeSummaryBox(pdf, sections.Standards)
	}
	writeMDSection(pdf, filepath.Join(reportsDir, "standards.md"))

	// Section 3: Doc Consistency
	pdf.AddPage()
	writeSectionHeader(pdf, "3. Documentation Consistency")
	if sections.DocConsistency != "" {
		writeSummaryBox(pdf, sections.DocConsistency)
	}
	writeMDSection(pdf, filepath.Join(reportsDir, "doc_consistency.md"))

	// Section 4: Protocol Recommendations
	pdf.AddPage()
	writeSectionHeader(pdf, "4. Protocol Recommendations")
	if sections.Protocol != "" {
		writeSummaryBox(pdf, sections.Protocol)
	}
	writeProtocolSummary(pdf, filepath.Join(reportsDir, "protocol_recommendation.md"))

	// Section 5: Enrichment
	pdf.AddPage()
	writeSectionHeader(pdf, "5. Documentation Enrichment")
	if sections.Enrichment != "" {
		writeSummaryBox(pdf, sections.Enrichment)
	}
	writeMDSection(pdf, filepath.Join(reportsDir, "enrichment_summary.md"))

	return pdf.OutputFileAndClose(pdfPath)
}

func writeSectionHeader(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetFillColor(41, 65, 122)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(0, 10, "  "+title, "", 1, "L", true, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)
}

func writeSummaryBox(pdf *gofpdf.Fpdf, text string) {
	pdf.SetFont("Helvetica", "I", 10)
	pdf.SetFillColor(245, 248, 255)
	x := pdf.GetX()
	y := pdf.GetY()
	w := 190.0
	// Calculate height needed
	lines := pdf.SplitText(sanitize(text), w-10)
	h := float64(len(lines))*5 + 8
	pdf.Rect(x, y, w, h, "F")
	pdf.SetXY(x+5, y+4)
	pdf.MultiCell(w-10, 5, sanitize(text), "", "L", false)
	pdf.SetY(y + h + 4)
	pdf.SetFont("Helvetica", "", 10)
}

func writeMDSection(pdf *gofpdf.Fpdf, mdPath string) {
	content, err := os.ReadFile(mdPath)
	if err != nil {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 6, "(Report not available)", "", 1, "L", false, 0, "")
		return
	}

	lines := strings.Split(string(content), "\n")
	inTable := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Skip markdown headers (we already have section headers)
		if strings.HasPrefix(line, "# ") {
			continue
		}

		// Sub-headers
		if strings.HasPrefix(line, "## ") {
			pdf.Ln(3)
			pdf.SetFont("Helvetica", "B", 12)
			pdf.CellFormat(0, 7, sanitize(strings.TrimPrefix(line, "## ")), "", 1, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 10)
			continue
		}

		// Table separator lines
		if strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "|---") {
			continue
		}

		// Table rows
		if strings.HasPrefix(line, "|") {
			if !inTable {
				inTable = true
			}
			writeTableRow(pdf, line, inTable)
			continue
		}

		if inTable && !strings.HasPrefix(line, "|") {
			inTable = false
			pdf.Ln(2)
		}

		// Bold text
		if strings.Contains(line, "**") {
			writeBoldLine(pdf, line)
			continue
		}

		// Bullet points
		if strings.HasPrefix(line, "- ") {
			pdf.SetFont("Helvetica", "", 9)
			pdf.CellFormat(5, 5, "", "", 0, "L", false, 0, "")
			writeWrapped(pdf, strings.TrimPrefix(line, "- "))
			continue
		}

		// Regular text
		if strings.TrimSpace(line) == "" {
			pdf.Ln(2)
			continue
		}

		pdf.SetFont("Helvetica", "", 9)
		writeWrapped(pdf, line)
	}
}

func writeProtocolSummary(pdf *gofpdf.Fpdf, mdPath string) {
	content, err := os.ReadFile(mdPath)
	if err != nil {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 6, "(Report not available)", "", 1, "L", false, 0, "")
		return
	}

	lines := strings.Split(string(content), "\n")
	rowCount := 0

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, "# ") {
			continue
		}

		if strings.HasPrefix(line, "## ") {
			pdf.Ln(3)
			pdf.SetFont("Helvetica", "B", 12)
			pdf.CellFormat(0, 7, sanitize(strings.TrimPrefix(line, "## ")), "", 1, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9)
			rowCount = 0
			continue
		}

		if strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "|---") {
			continue
		}

		if strings.HasPrefix(line, "|") {
			// Limit per-endpoint table to first 30 rows to keep PDF manageable
			rowCount++
			if rowCount > 32 {
				if rowCount == 33 {
					pdf.SetFont("Helvetica", "I", 8)
					pdf.CellFormat(0, 5, "(... remaining endpoints omitted for brevity)", "", 1, "L", false, 0, "")
				}
				continue
			}
			writeTableRow(pdf, line, true)
			continue
		}

		if strings.Contains(line, "**") {
			writeBoldLine(pdf, line)
			continue
		}

		if strings.HasPrefix(line, "- ") {
			pdf.SetFont("Helvetica", "", 9)
			pdf.CellFormat(5, 5, "", "", 0, "L", false, 0, "")
			writeWrapped(pdf, strings.TrimPrefix(line, "- "))
			continue
		}

		if strings.TrimSpace(line) != "" {
			pdf.SetFont("Helvetica", "", 9)
			writeWrapped(pdf, line)
		}
	}
}

func writeTableRow(pdf *gofpdf.Fpdf, line string, isData bool) {
	cells := strings.Split(line, "|")
	var cleaned []string
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if len(cleaned) == 0 {
		return
	}

	pageW := 190.0
	colW := pageW / float64(len(cleaned))
	if colW > 60 {
		colW = 60
	}

	fontSize := 7.0
	if !isData {
		fontSize = 8.0
	}
	pdf.SetFont("Helvetica", "", fontSize)

	for i, cell := range cleaned {
		w := colW
		if i == len(cleaned)-1 {
			w = 0 // last column takes remaining width
		}
		// Strip backticks and emojis for PDF
		cell = stripMarkdown(cell)
		if len(cell) > 80 {
			cell = cell[:77] + "..."
		}
		border := ""
		pdf.CellFormat(w, 4.5, sanitize(cell), border, 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

func writeBoldLine(pdf *gofpdf.Fpdf, line string) {
	parts := strings.Split(line, "**")
	for i, part := range parts {
		if i%2 == 1 {
			pdf.SetFont("Helvetica", "B", 10)
		} else {
			pdf.SetFont("Helvetica", "", 10)
		}
		if part != "" {
			pdf.Write(5, sanitize(part))
		}
	}
	pdf.Ln(5)
}

func writeWrapped(pdf *gofpdf.Fpdf, text string) {
	pdf.MultiCell(0, 5, sanitize(text), "", "L", false)
}

func stripMarkdown(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	return s
}

// sanitize removes non-latin1 characters that gofpdf can't handle.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r <= 255 {
			b.WriteRune(r)
		} else {
			// Replace emojis/unicode with a space or skip
			switch {
			case r >= 0x1F300 && r <= 0x1FAFF: // emoji range
				// skip emojis
			default:
				b.WriteRune(' ')
			}
		}
		i += size
	}
	return b.String()
}

// readMDFile reads a markdown file and returns its lines.
func readMDFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
