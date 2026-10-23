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
	_ = f.SetSheetName("Sheet1", "strings")
	_ = f.SetCellStr("strings", "A1", "id")
	_ = f.SetCellStr("strings", "B1", "text")
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetCellStr("strings", cell, row.ID)
		cell, _ = excelize.CoordinatesToCellName(2, i+2)
		_ = f.SetCellStr("strings", cell, row.Text)
	}
	// Serialization of this fresh, fixed-schema, unencrypted workbook has no
	// external writer and no user-selected serialization options that can fail.
	b, _ := f.WriteToBuffer()
	return b.Bytes(), nil
}

func readStrings(raw []byte) (map[string]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("strings.xlsx: %w", err)
	}
	defer func() { _ = f.Close() }() // Reader owns only the supplied in-memory XLSX bytes.
	rows, err := f.GetRows("strings")
	if err != nil {
		return nil, fmt.Errorf("strings.xlsx: %w", err)
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "id" || rows[0][1] != "text" {
		return nil, fmt.Errorf("strings.xlsx requires id/text columns")
	}
	out := make(map[string]string, len(rows)-1)
	for i, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		if row[0] == "" || len(row) > 2 {
			return nil, fmt.Errorf("invalid strings.xlsx row %d", i+2)
		}
		if _, ok := out[row[0]]; ok {
			return nil, fmt.Errorf("duplicate string id %q", row[0])
		}
		text := ""
		if len(row) == 2 {
			text = row[1]
		}
		for col := 1; col <= 2; col++ {
			cell, _ := excelize.CoordinatesToCellName(col, i+2)
			formula, _ := f.GetCellFormula("strings", cell)
			if formula != "" {
				return nil, fmt.Errorf("formula in strings.xlsx cell %s", cell)
			}
		}
		out[row[0]] = text
	}
	return out, nil
}
