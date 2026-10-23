package exe

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

const (
	fontSheetName      = "Шрифт"
	fontReservedSymbol = "служебный"
)

func addFontSheet(raw []byte, font GameFont) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("strings.xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()           // The workbook owns only in-memory input bytes.
	index, _ := f.GetSheetIndex(fontSheetName) // The fixed sheet name is valid.
	if index >= 0 {
		return nil, fmt.Errorf("strings.xlsx already contains sheet %s", fontSheetName)
	}
	// The sheet name and all coordinates are fixed and valid; this newly created
	// sheet cannot contain malformed user cells. The supplied font is validated.
	_, _ = f.NewSheet(fontSheetName)
	_ = f.SetCellStr(fontSheetName, "A1", "number")
	_ = f.SetCellStr(fontSheetName, "B1", "symbol")
	symbols := make(map[int]string, 96)
	for _, glyph := range font.Glyphs {
		symbols[glyph.Slot] = glyph.Unicode
	}
	symbols[127] = fontReservedSymbol
	for slot := 32; slot <= 127; slot++ {
		row := strconv.Itoa(slot - 30)
		_ = f.SetCellInt(fontSheetName, "A"+row, int64(slot-31))
		_ = f.SetCellStr(fontSheetName, "B"+row, symbols[slot])
	}
	// Streaming readers use these bounds; the fresh sheet and fixed range are valid.
	_ = f.SetSheetDimension(fontSheetName, "A1:B97")
	// Serialization uses Excelize's default in-memory ZIP writer without streams,
	// encryption or a user-supplied writer, so there is no fallible external sink.
	b, _ := f.WriteToBuffer()
	return b.Bytes(), nil
}

func readFontSheet(raw []byte, font *GameFont) error {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("strings.xlsx: %w", err)
	}
	defer func() { _ = f.Close() }() // Reader owns only the supplied in-memory XLSX bytes.
	rows, err := f.GetRows(fontSheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return fmt.Errorf("strings.xlsx sheet %s: %w", fontSheetName, err)
	}
	for i, row := range rows {
		if len(row) > 2 {
			return fmt.Errorf("strings.xlsx sheet %s row %d requires only number/symbol columns", fontSheetName, i+1)
		}
		for _, column := range []string{"A", "B"} {
			cell := column + strconv.Itoa(i+1)
			// GetRows has already loaded this sheet; both coordinates are valid.
			formula, _ := f.GetCellFormula(fontSheetName, cell)
			if formula != "" {
				return fmt.Errorf("formula in strings.xlsx sheet %s cell %s", fontSheetName, cell)
			}
		}
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "number" || rows[0][1] != "symbol" {
		return fmt.Errorf("strings.xlsx sheet %s requires number/symbol columns", fontSheetName)
	}
	if len(rows) != 97 {
		return fmt.Errorf("strings.xlsx sheet %s requires 96 cell numbers (1..96), including reserved cell number 96", fontSheetName)
	}
	symbols := make(map[int]string, 96)
	unicodeSlots := make(map[rune]bool, 95)
	for i, row := range rows[1:] {
		if len(row) != 2 {
			return fmt.Errorf("strings.xlsx sheet %s row %d requires number/symbol columns", fontSheetName, i+2)
		}
		number, err := strconv.Atoi(row[0])
		if err != nil || number < 1 || number > 96 || strconv.Itoa(number) != row[0] || symbols[number+31] != "" {
			return fmt.Errorf("invalid or duplicate cell number %q in strings.xlsx sheet %s row %d; use 1..96", row[0], fontSheetName, i+2)
		}
		slot := number + 31
		if number == 96 {
			if row[1] != fontReservedSymbol {
				return fmt.Errorf("reserved cell number 96 in strings.xlsx sheet %s must keep the symbol label %q", fontSheetName, fontReservedSymbol)
			}
			symbols[slot] = row[1]
			continue
		}
		r, size := utf8.DecodeRuneInString(row[1])
		if size != len(row[1]) || r == utf8.RuneError || !unicode.IsPrint(r) {
			return fmt.Errorf("invalid symbol for cell number %d in strings.xlsx sheet %s; use one printable Unicode character", number, fontSheetName)
		}
		if (slot == 32 || slot == '_' || (slot >= '0' && slot <= '9')) && row[1] != string(rune(slot)) {
			return fmt.Errorf("cell number %d in strings.xlsx sheet %s is reserved for its original space/digit/underscore symbol", number, fontSheetName)
		}
		if unicodeSlots[r] {
			return fmt.Errorf("duplicate Unicode symbol %q for cell number %d in strings.xlsx sheet %s", r, number, fontSheetName)
		}
		symbols[slot] = row[1]
		unicodeSlots[r] = true
	}
	// All 96 cell numbers and 95 printable symbols have been checked before changing the font.
	for i := range font.Glyphs {
		if font.Glyphs[i].Slot != 127 {
			font.Glyphs[i].Unicode = symbols[font.Glyphs[i].Slot]
		}
	}
	return nil
}
