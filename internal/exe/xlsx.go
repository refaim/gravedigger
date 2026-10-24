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
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetCellStr("Тексты", cell, row.Text)
		cell, _ = excelize.CoordinatesToCellName(2, i+1)
		_ = f.SetCellStr("Тексты", cell, "")
	}
	lastCell, _ := excelize.CoordinatesToCellName(2, max(1, len(rows)))
	_ = f.SetColWidth("Тексты", "A", "B", 60)
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
		if len(row) > 2 {
			return nil, fmt.Errorf("strings.xlsx cell C%d: only original/translation columns A/B are allowed", i+1)
		}
		for col := 1; col <= 2; col++ {
			cell, _ := excelize.CoordinatesToCellName(col, i+1)
			// GetRows loaded this sheet, and these bounded coordinates are valid.
			formula, _ := f.GetCellFormula(sheet, cell)
			if formula != "" {
				return nil, fmt.Errorf("formula in strings.xlsx cell %s", cell)
			}
		}
	}
	// A legacy workbook has one extra ID/text header row. Checking its size also
	// leaves a real first original "id" with translation "text" unambiguous.
	if len(rows) > len(expected) && len(rows[0]) == 2 && rows[0][0] == "id" && rows[0][1] == "text" {
		return readLegacyStringRows(rows[1:], expected)
	}
	if len(rows) > len(expected) {
		return nil, fmt.Errorf("strings.xlsx has extra rows: expected %d original/translation rows, got %d", len(expected), len(rows))
	}
	out := make(map[string]string, len(expected))
	for i, original := range expected {
		text, translation := "", ""
		if i < len(rows) {
			if len(rows[i]) > 0 {
				text = rows[i][0]
			}
			if len(rows[i]) > 1 {
				translation = rows[i][1]
			}
		}
		if text != original.Text {
			return nil, fmt.Errorf("strings.xlsx cell A%d: original text is missing, reordered or changed; keep column A unchanged", i+1)
		}
		if translation == "" {
			translation = original.Text
		}
		out[original.ID] = translation
	}
	return out, nil
}

func readLegacyStringRows(rows [][]string, expected []StringRow) (map[string]string, error) {
	known := make(map[string]bool, len(expected))
	for _, original := range expected {
		known[original.ID] = true
	}
	out := make(map[string]string, len(expected))
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		if row[0] == "" {
			return nil, fmt.Errorf("invalid legacy strings.xlsx cell A%d: missing string id", i+2)
		}
		if !known[row[0]] {
			return nil, fmt.Errorf("unknown legacy strings.xlsx id %q", row[0])
		}
		if _, duplicate := out[row[0]]; duplicate {
			return nil, fmt.Errorf("duplicate legacy strings.xlsx id %q", row[0])
		}
		text := ""
		if len(row) == 2 {
			text = row[1]
		}
		out[row[0]] = text
	}
	for _, original := range expected {
		if _, ok := out[original.ID]; !ok {
			return nil, fmt.Errorf("missing legacy strings.xlsx id %q", original.ID)
		}
	}
	return out, nil
}
