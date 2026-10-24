package exe

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

type StringRow struct{ ID, Text string }

func writeStrings(rows []StringRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }() // In-memory workbook; there is no external file to flush.
	// The workbook and all coordinates are created here, so these calls cannot
	// refer to a missing sheet or an invalid cell.
	_ = f.SetSheetName("Sheet1", "Тексты")
	for i, header := range []string{"ID", "Английский", "Русский"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr("Тексты", cell, header)
	}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetCellStr("Тексты", cell, row.ID)
		cell, _ = excelize.CoordinatesToCellName(2, i+2)
		_ = f.SetCellStr("Тексты", cell, row.Text)
		cell, _ = excelize.CoordinatesToCellName(3, i+2)
		_ = f.SetCellStr("Тексты", cell, "")
	}
	lastCell, _ := excelize.CoordinatesToCellName(3, len(rows)+1)
	_ = f.SetColWidth("Тексты", "A", "A", 32)
	_ = f.SetColWidth("Тексты", "B", "C", 60)
	// The alignment and range are constants on this freshly created worksheet.
	style, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	_ = f.SetCellStyle("Тексты", "A1", lastCell, style)
	_ = f.SetSheetDimension("Тексты", "A1:"+lastCell)
	// Serialization of this fresh, fixed-schema, unencrypted workbook has no
	// external writer and no user-selected serialization options that can fail.
	b, _ := f.WriteToBuffer()
	return b.Bytes(), nil
}

func readStrings(raw []byte, expected []StringRow) (map[string]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("strings.xlsx: %w", err)
	}
	defer func() { _ = f.Close() }() // Reader owns only the supplied in-memory XLSX bytes.
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("strings.xlsx: %w", err)
	}
	for i, row := range rows {
		if len(row) > 3 {
			return nil, fmt.Errorf("strings.xlsx cell D%d: only ID/original/translation columns A/B/C are allowed", i+1)
		}
		for col := 1; col <= 3; col++ {
			cell, _ := excelize.CoordinatesToCellName(col, i+1)
			// GetRows loaded this sheet, and these bounded coordinates are valid.
			formula, _ := f.GetCellFormula(sheet, cell)
			if formula != "" {
				return nil, fmt.Errorf("formula in strings.xlsx cell %s", cell)
			}
		}
	}
	if len(rows) == 0 || len(rows[0]) != 3 || rows[0][0] != "ID" || rows[0][1] != "Английский" || rows[0][2] != "Русский" {
		return nil, fmt.Errorf("strings.xlsx cell A1: expected headers ID, Английский, Русский")
	}
	return readIDStringRows(rows[1:], expected)
}

func readIDStringRows(rows [][]string, expected []StringRow) (map[string]string, error) {
	known := make(map[string]string, len(expected))
	for _, original := range expected {
		known[original.ID] = original.Text
	}
	out := make(map[string]string, len(expected))
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		if row[0] == "" {
			return nil, fmt.Errorf("invalid strings.xlsx cell A%d: missing string id", i+2)
		}
		original, exists := known[row[0]]
		if !exists {
			return nil, fmt.Errorf("unknown strings.xlsx id %q in cell A%d", row[0], i+2)
		}
		if _, duplicate := out[row[0]]; duplicate {
			return nil, fmt.Errorf("duplicate strings.xlsx id %q in cell A%d", row[0], i+2)
		}
		text := ""
		if len(row) > 1 {
			text = row[1]
		}
		if text != original {
			return nil, fmt.Errorf("strings.xlsx cell B%d: original English text for id %q is missing or changed", i+2, row[0])
		}
		translation := ""
		if len(row) > 2 {
			translation = row[2]
		}
		if translation == "" {
			translation = original
		}
		out[row[0]] = translation
	}
	for _, original := range expected {
		if _, ok := out[original.ID]; !ok {
			return nil, fmt.Errorf("missing strings.xlsx id %q", original.ID)
		}
	}
	return out, nil
}
