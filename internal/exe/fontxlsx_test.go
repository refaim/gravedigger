package exe

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func fontSheetTestFont() GameFont {
	f := GameFont{Version: 1, Height: 10}
	for slot := 32; slot <= 127; slot++ {
		f.Glyphs = append(f.Glyphs, GameGlyph{Slot: slot, Unicode: string(rune(slot)), Width: 1, Pixels: make([]uint16, 10), Shadow: make([]uint16, 10)})
	}
	return f
}

func editFontWorkbook(t *testing.T, raw []byte, edit func(*excelize.File) error) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := edit(f); err != nil {
		t.Fatal(err)
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func fontWorkbook(t *testing.T) []byte {
	t.Helper()
	raw, err := writeStrings([]StringRow{{ID: "title", Text: "=Привет"}, {ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = addFontSheet(raw, fontSheetTestFont())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFontSheetRoundtrip(t *testing.T) {
	want := fontSheetTestFont()
	raw, err := writeStrings([]StringRow{{ID: "title", Text: "=Привет"}, {ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	raw = editFontWorkbook(t, raw, func(f *excelize.File) error {
		if _, err := f.NewSheet("other"); err != nil {
			return err
		}
		return f.SetCellStr("other", "C4", "keep me")
	})
	for i, j := 0, len(want.Glyphs)-1; i < j; i, j = i+1, j-1 {
		want.Glyphs[i], want.Glyphs[j] = want.Glyphs[j], want.Glyphs[i]
	}
	raw, err = addFontSheet(raw, want)
	if err != nil {
		t.Fatal(err)
	}
	got := fontSheetTestFont()
	before := fontSheetTestFont()
	if err := readFontSheet(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatal("no-op changed font geometry or mappings")
	}
	values, err := readStrings(raw)
	if err != nil || values["title"] != "=Привет" || values["empty"] != "" {
		t.Fatalf("strings changed: %v, %v", values, err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	dimension, err := f.GetSheetDimension("Шрифт")
	if err != nil || dimension != "A1:B97" {
		t.Fatalf("streaming readers need the complete font sheet dimension, got %q: %v", dimension, err)
	}
	rows, err := f.GetRows("Шрифт", excelize.Options{RawCellValue: true})
	if err != nil || len(rows) != 97 || rows[0][0] != "number" || rows[0][1] != "symbol" || rows[1][1] != " " || rows[96][0] != "96" || rows[96][1] != "служебный" {
		t.Fatalf("unexpected sheet: %v, %v", rows, err)
	}
	for number, row := range rows[1:] {
		if row[0] != strconv.Itoa(number+1) {
			t.Fatalf("wrong visible number in row %d: %q", number+2, row[0])
		}
	}
	value, err := f.GetCellValue("other", "C4")
	if err != nil || value != "keep me" {
		t.Fatalf("other sheet changed: %q, %v", value, err)
	}
}

func TestFontSheetEditableMappingAndOrder(t *testing.T) {
	raw := editFontWorkbook(t, fontWorkbook(t), func(f *excelize.File) error {
		for _, row := range []struct{ cell, value string }{{"A2", "34"}, {"B2", "Я"}, {"A35", "1"}, {"B35", " "}} {
			if err := f.SetCellStr("Шрифт", row.cell, row.value); err != nil {
				return err
			}
		}
		// A display format must not change the raw symbol or visible number.
		style, err := f.NewStyle(&excelize.Style{NumFmt: 1})
		if err != nil {
			return err
		}
		return f.SetCellStyle("Шрифт", "A2", "A2", style)
	})
	f := fontSheetTestFont()
	for i, j := 0, len(f.Glyphs)-1; i < j; i, j = i+1, j-1 {
		f.Glyphs[i], f.Glyphs[j] = f.Glyphs[j], f.Glyphs[i]
	}
	want := fontSheetTestFont()
	want.Glyphs[65-32].Unicode = "Я"
	for i, j := 0, len(want.Glyphs)-1; i < j; i, j = i+1, j-1 {
		want.Glyphs[i], want.Glyphs[j] = want.Glyphs[j], want.Glyphs[i]
	}
	if err := readFontSheet(raw, &f); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, want) {
		t.Fatal("mapping import changed other font data or relied on row/glyph order")
	}
}

func TestFontSheetReserved96(t *testing.T) {
	valid := fontWorkbook(t)
	shuffled := editFontWorkbook(t, valid, func(f *excelize.File) error {
		for _, row := range []struct{ cell, value string }{{"A2", "96"}, {"B2", "служебный"}, {"A97", "1"}, {"B97", " "}} {
			if err := f.SetCellStr("Шрифт", row.cell, row.value); err != nil {
				return err
			}
		}
		return nil
	})
	f := fontSheetTestFont()
	if err := readFontSheet(shuffled, &f); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, fontSheetTestFont()) {
		t.Fatal("moving reserved row changed font, including the original DEL mapping")
	}
	duplicate := editFontWorkbook(t, valid, func(f *excelize.File) error {
		if err := f.SetCellStr("Шрифт", "A2", "96"); err != nil {
			return err
		}
		return f.SetCellStr("Шрифт", "B2", "служебный")
	})
	if err := readFontSheet(duplicate, &f); err == nil || !strings.Contains(err.Error(), "duplicate cell number") || !strings.Contains(err.Error(), "96") {
		t.Fatalf("duplicate reserved number: %v", err)
	}
}

func TestFontSheetRejectsInvalidWorkbooks(t *testing.T) {
	valid := fontWorkbook(t)
	for _, tc := range []struct {
		name, cell, value, message string
	}{
		{"header number", "A1", "id", "columns"},
		{"old header", "A1", "slot", "columns"},
		{"header symbol", "B1", "text", "columns"},
		{"extra header", "C1", "extra", "columns"},
		{"extra column", "C2", "extra", "columns"},
		{"missing number", "A2", "", "number"},
		{"duplicate number", "A3", "1", "number"},
		{"low number", "A2", "0", "number"},
		{"duplicate control number", "A97", "1", "number"},
		{"high number", "A2", "97", "number"},
		{"non-number", "A2", "x", "number"},
		{"fraction", "A2", "1.5", "number"},
		{"leading zero", "A2", "01", "number"},
		{"plus", "A2", "+1", "number"},
		{"empty symbol", "B3", "", "symbol"},
		{"many symbols", "B3", "AB", "symbol"},
		{"control symbol", "B3", "\n", "symbol"},
		{"DEL", "B3", "\x7f", "symbol"},
		{"C1 control", "B3", "\u0085", "symbol"},
		{"replacement", "B3", "\ufffd", "symbol"},
		{"zero-width", "B3", "\u200b", "symbol"},
		{"duplicate symbol", "B3", "A", "duplicate"},
		{"space reserved", "B2", "Я", "number 1"},
		{"digit reserved", "B18", "Я", "number 17"},
		{"underscore reserved", "B65", "Я", "number 64"},
		{"control label", "B97", "Я", "number 96"},
		{"control DEL", "B97", "\x7f", "number 96"},
		{"control whitespace", "B97", "служебный ", "number 96"},
		{"control empty", "B97", "", "symbol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := editFontWorkbook(t, valid, func(f *excelize.File) error {
				return f.SetCellStr("Шрифт", tc.cell, tc.value)
			})
			f := fontSheetTestFont()
			before := fontSheetTestFont()
			err := readFontSheet(raw, &f)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v, want %q", err, tc.message)
			}
			if strings.Contains(err.Error(), "slot") {
				t.Fatalf("native numbering leaked into a workbook error: %v", err)
			}
			if !reflect.DeepEqual(f, before) {
				t.Fatal("invalid import partially changed font")
			}
		})
	}
	for _, cell := range []string{"A1", "B1", "A2", "B2", "C2", "A97", "B97"} {
		t.Run("formula "+cell, func(t *testing.T) {
			raw := editFontWorkbook(t, valid, func(f *excelize.File) error {
				return f.SetCellFormula("Шрифт", cell, "1+1")
			})
			f := fontSheetTestFont()
			if err := readFontSheet(raw, &f); err == nil {
				t.Fatal("accepted formula")
			}
		})
	}
	for _, count := range []int{0, 95, 97} {
		t.Run("row count "+strconv.Itoa(count), func(t *testing.T) {
			rows := [][]any{{"number", "symbol"}}
			for i := 0; i < count; i++ {
				rows = append(rows, []any{i + 1, string(rune(i + 32))})
			}
			f := fontSheetTestFont()
			if err := readFontSheet(badWorkbook(t, rows, "Шрифт"), &f); err == nil || !strings.Contains(err.Error(), "1..96") {
				t.Fatalf("wrong row count must identify visible numbers 1..96: %v", err)
			}
		})
	}
	f := fontSheetTestFont()
	if err := readFontSheet(badWorkbook(t, nil, "Шрифт"), &f); err == nil {
		t.Fatal("accepted empty sheet")
	}
	if err := readFontSheet(badWorkbook(t, [][]any{{"number"}}, "Шрифт"), &f); err == nil {
		t.Fatal("accepted incomplete headers")
	}
	blankRow := editFontWorkbook(t, valid, func(f *excelize.File) error {
		if err := f.SetCellStr("Шрифт", "A3", ""); err != nil {
			return err
		}
		return f.SetCellStr("Шрифт", "B3", "")
	})
	if err := readFontSheet(blankRow, &f); err == nil {
		t.Fatal("accepted blank internal row")
	}
	if err := readFontSheet(badWorkbook(t, nil, "other"), &f); err == nil || !strings.Contains(err.Error(), "Шрифт") {
		t.Fatalf("missing sheet: %v", err)
	}
	if err := readFontSheet([]byte("not zip"), &f); err == nil {
		t.Fatal("accepted corrupt workbook")
	}
	if _, err := addFontSheet([]byte("not zip"), f); err == nil {
		t.Fatal("added sheet to corrupt workbook")
	}
	if _, err := addFontSheet(valid, f); err == nil {
		t.Fatal("overwrote existing font sheet")
	}
}
