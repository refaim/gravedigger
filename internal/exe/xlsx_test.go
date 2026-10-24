package exe

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteStringsXLSXLayout(t *testing.T) {
	for _, rows := range [][]StringRow{
		nil,
		{{ID: "message_id", Text: "  Original\ntext  "}, {ID: "formula_id", Text: "=1+1"}, {ID: "empty_id"}},
	} {
		raw, err := writeStrings(rows)
		if err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if f.GetSheetName(0) != "Тексты" {
			t.Fatal("first sheet must be Тексты", f.GetSheetList())
		}
		dimension, err := f.GetSheetDimension("Тексты")
		if want := "A1:C" + strconv.Itoa(len(rows)+1); err != nil || dimension != want {
			t.Fatalf("dimension %q, want %q: %v", dimension, want, err)
		}
		for i, header := range []string{"ID", "Английский", "Русский"} {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			if value, err := f.GetCellValue("Тексты", cell); err != nil || value != header {
				t.Fatalf("header %s: %q, want %q: %v", cell, value, header, err)
			}
		}
		for _, column := range []string{"A", "B", "C"} {
			wantWidth := float64(60)
			if column == "A" {
				wantWidth = 32
			}
			width, err := f.GetColWidth("Тексты", column)
			if err != nil || width != wantWidth {
				t.Fatalf("column %s width %v, want %v: %v", column, width, wantWidth, err)
			}
			for _, row := range []int{1, len(rows) + 1} {
				cell := column + strconv.Itoa(row)
				styleID, err := f.GetCellStyle("Тексты", cell)
				if err != nil {
					t.Fatal(err)
				}
				style, err := f.GetStyle(styleID)
				if err != nil || style.Alignment == nil || !style.Alignment.WrapText {
					t.Fatalf("cell %s must wrap text: %v, %v", cell, style, err)
				}
			}
		}
		for i, row := range rows {
			a, b, c := "A"+strconv.Itoa(i+2), "B"+strconv.Itoa(i+2), "C"+strconv.Itoa(i+2)
			id, err := f.GetCellValue("Тексты", a, excelize.Options{RawCellValue: true})
			if err != nil || id != row.ID {
				t.Fatalf("ID row %d: %q, %v", i+2, id, err)
			}
			original, err := f.GetCellValue("Тексты", b, excelize.Options{RawCellValue: true})
			if err != nil || original != row.Text {
				t.Fatalf("original row %d: %q, %v", i+1, original, err)
			}
			translation, err := f.GetCellValue("Тексты", c, excelize.Options{RawCellValue: true})
			if err != nil || translation != "" {
				t.Fatalf("translation row %d must start empty: %q, %v", i+1, translation, err)
			}
			if formula, err := f.GetCellFormula("Тексты", b); err != nil || formula != "" {
				t.Fatal("an original beginning with = must remain text", formula, err)
			}
		}
	}
}

func TestStringsXLSX(t *testing.T) {
	want := []StringRow{{ID: "title", Text: "Dangerous Dave"}, {ID: "empty", Text: ""}, {ID: "unicode", Text: "Привет\nмир"}, {ID: "formula_text", Text: "=1+1"}}
	raw, err := writeStrings(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readStrings(raw, want)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range want {
		if got[row.ID] != row.Text {
			t.Fatalf("%s: %q", row.ID, got[row.ID])
		}
	}
}

func editStringsWorkbook(t *testing.T, raw []byte, edit func(*excelize.File) error) []byte {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := edit(f); err != nil {
		t.Fatal(err)
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStringsXLSXTranslationColumnsAndRenamedSheet(t *testing.T) {
	expected := []StringRow{{"first", "Same"}, {"second", "Same"}, {"blank", ""}, {"spaces", "  original  "}, {"formula_literal", "=original"}, {"trailing1", ""}, {"trailing2", ""}}
	raw, err := writeStrings(expected)
	if err != nil {
		t.Fatal(err)
	}
	raw = editStringsWorkbook(t, raw, func(f *excelize.File) error {
		if err := f.SetSheetName("Тексты", "SP renamed this"); err != nil {
			return err
		}
		for i, translation := range []string{"Первый", "  Второй\n ", " ", "", "=literal translation"} {
			if err := f.SetCellStr("SP renamed this", "C"+strconv.Itoa(i+2), translation); err != nil {
				return err
			}
		}
		index, err := f.NewSheet("Шрифт")
		if err != nil {
			return err
		}
		f.SetActiveSheet(index)
		return f.SetCellStr("Шрифт", "A1", "not the text sheet")
	})
	got, err := readStrings(raw, expected)
	want := map[string]string{"first": "Первый", "second": "  Второй\n ", "blank": " ", "spaces": "  original  ", "formula_literal": "=literal translation", "trailing1": "", "trailing2": ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("translations, duplicate originals, whitespace, or trailing empty rows lost", got, err)
	}
}

func TestStringsXLSXThreeColumnsBindByID(t *testing.T) {
	expected := []StringRow{{"first", "Same"}, {"second", "Same"}}
	raw := badWorkbook(t, [][]any{{"ID", "Английский", "Русский"}, {"second", "Same", " Второй "}, {}, {"first", "Same", "Первый"}}, "renamed")
	got, err := readStrings(raw, expected)
	want := map[string]string{"first": "Первый", "second": " Второй "}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("reordered rows with duplicate originals must bind translations by ID", got, err)
	}
}

func TestStringsXLSXRejectsOldFormats(t *testing.T) {
	expected := []StringRow{{"x", "First"}, {"y", "Second"}}
	for name, rows := range map[string][][]any{
		"old ID text":                     {{"id", "text"}, {"x", "Первый"}, {"y", "Второй"}},
		"headerless original translation": {{"First", "Первый"}, {"Second", "Второй"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readStrings(badWorkbook(t, rows, "Тексты"), expected); err == nil {
				t.Fatal("old workbook formats must be rejected")
			}
		})
	}
}

func TestStringsXLSXRejectsInvalidThreeColumns(t *testing.T) {
	expected := []StringRow{{"x", "First"}, {"y", "Second"}}
	for name, rows := range map[string][][]any{
		"wrong ID header":      {{"wrong", "Английский", "Русский"}, {"x", "First"}, {"y", "Second"}},
		"wrong English header": {{"ID", "wrong", "Русский"}, {"x", "First"}, {"y", "Second"}},
		"wrong Russian header": {{"ID", "Английский", "wrong"}, {"x", "First"}, {"y", "Second"}},
		"missing ID":           {{"ID", "Английский", "Русский"}, {"x", "First"}},
		"duplicate ID":         {{"ID", "Английский", "Русский"}, {"x", "First"}, {"x", "First"}},
		"unknown ID":           {{"ID", "Английский", "Русский"}, {"unknown", "First"}, {"y", "Second"}},
		"empty ID":             {{"ID", "Английский", "Русский"}, {"", "First"}, {"y", "Second"}},
		"wrong original":       {{"ID", "Английский", "Русский"}, {"x", "Changed"}, {"y", "Second"}},
		"missing original":     {{"ID", "Английский", "Русский"}, {"x"}, {"y", "Second"}},
		"other ID original":    {{"ID", "Английский", "Русский"}, {"y", "First"}, {"x", "Second"}},
		"fourth column":        {{"ID", "Английский", "Русский"}, {"x", "First", "translation", "extra"}, {"y", "Second"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readStrings(badWorkbook(t, rows, "Тексты"), expected); err == nil {
				t.Fatal("accepted invalid three-column workbook")
			}
		})
	}
	for _, cell := range []string{"A1", "B1", "C1", "A2", "B2", "C2"} {
		t.Run("formula "+cell, func(t *testing.T) {
			raw := badWorkbook(t, [][]any{{"ID", "Английский", "Русский"}, {"x", "First"}, {"y", "Second"}}, "Тексты")
			raw = editStringsWorkbook(t, raw, func(f *excelize.File) error { return f.SetCellFormula("Тексты", cell, "1+1") })
			if _, err := readStrings(raw, expected); err == nil || !strings.Contains(err.Error(), "formula") {
				t.Fatal("formula cells must never be imported by their cached value", err)
			}
		})
	}
}

func TestStringsXLSXEmptyOriginals(t *testing.T) {
	for _, expected := range [][]StringRow{nil, {{"a", ""}}, {{"a", ""}, {"b", ""}}} {
		raw, err := writeStrings(expected)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readStrings(raw, expected)
		if err != nil || len(got) != len(expected) {
			t.Fatal("empty originals must remain valid with their IDs", got, err)
		}
	}
	for _, rows := range [][][]any{
		{{"ID", "Английский", "Русский"}, {"a", "First"}, {"b"}, {"c"}},
		{{"ID", "Английский", "Русский"}, {"a", "First"}, {"b", "", "Translated empty original"}, {"c"}},
	} {
		expected := []StringRow{{"a", "First"}, {"b", ""}, {"c", ""}}
		got, err := readStrings(badWorkbook(t, rows, "any name"), expected)
		if err != nil || got["a"] != "First" || got["c"] != "" {
			t.Fatal("empty English cells must remain valid while their IDs are present", got, err)
		}
	}
}

func TestStringsXLSXOriginalMayLookLikeOldHeader(t *testing.T) {
	expected := []StringRow{{"real_id", "id"}}
	got, err := readStrings(badWorkbook(t, [][]any{{"ID", "Английский", "Русский"}, {"real_id", "id", "text"}}, "Тексты"), expected)
	if err != nil || got["real_id"] != "text" {
		t.Fatal("ordinary original and translation strings must remain literal", got, err)
	}
}

func badWorkbook(t *testing.T, rows [][]any, sheet string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	if sheet != "Sheet1" {
		if err := f.SetSheetName("Sheet1", sheet); err != nil {
			t.Fatal(err)
		}
	}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStringsXLSXRejectsInvalid(t *testing.T) {
	expected := []StringRow{{"x", "First"}, {"y", "Second"}}
	if _, err := readStrings(badWorkbook(t, nil, "Тексты"), expected); err == nil {
		t.Fatal("accepted an empty sheet without the required headers")
	}
	if _, err := readStrings([]byte("not zip"), expected); err == nil {
		t.Fatal("accepted invalid XLSX bytes")
	}
	raw := badWorkbook(t, nil, "empty")
	raw = editStringsWorkbook(t, raw, func(f *excelize.File) error { f.WorkBook.Sheets.Sheet = nil; return nil })
	if _, err := readStrings(raw, expected); err == nil {
		t.Fatal("accepted workbook without a worksheet")
	}
}
