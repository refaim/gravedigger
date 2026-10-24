package exe

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteStringsXLSXLayout(t *testing.T) {
	for _, rows := range [][]StringRow{
		nil,
		{{ID: "hidden_id", Text: "  Original\ntext  "}, {ID: "hidden_formula_id", Text: "=1+1"}, {ID: "hidden_empty_id"}},
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
			t.Fatal("first sheet must be Тексты without technical headers", f.GetSheetList())
		}
		dimension, err := f.GetSheetDimension("Тексты")
		if want := "A1:B" + strconv.Itoa(max(1, len(rows))); err != nil || dimension != want {
			t.Fatalf("dimension %q, want %q: %v", dimension, want, err)
		}
		for _, column := range []string{"A", "B"} {
			width, err := f.GetColWidth("Тексты", column)
			if err != nil || width != 60 {
				t.Fatalf("column %s width %v, want 60: %v", column, width, err)
			}
			for _, row := range []int{1, max(1, len(rows))} {
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
			a, b := "A"+strconv.Itoa(i+1), "B"+strconv.Itoa(i+1)
			original, err := f.GetCellValue("Тексты", a, excelize.Options{RawCellValue: true})
			if err != nil || original != row.Text {
				t.Fatalf("original row %d: %q, %v", i+1, original, err)
			}
			translation, err := f.GetCellValue("Тексты", b, excelize.Options{RawCellValue: true})
			if err != nil || translation != "" {
				t.Fatalf("translation row %d must start empty: %q, %v", i+1, translation, err)
			}
			if formula, err := f.GetCellFormula("Тексты", a); err != nil || formula != "" {
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
			if err := f.SetCellStr("SP renamed this", "B"+strconv.Itoa(i+1), translation); err != nil {
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

func TestStringsXLSXEmptyOriginals(t *testing.T) {
	for _, expected := range [][]StringRow{nil, {{"a", ""}}, {{"a", ""}, {"b", ""}}} {
		raw, err := writeStrings(expected)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readStrings(raw, expected)
		if err != nil || len(got) != len(expected) {
			t.Fatal("empty originals are valid even when GetRows trims them", got, err)
		}
	}
	for _, rows := range [][][]any{{{"First"}}, {{"First"}, {"", "Translated empty original"}}} {
		expected := []StringRow{{"a", "First"}, {"b", ""}, {"c", ""}}
		got, err := readStrings(badWorkbook(t, rows, "any name"), expected)
		if err != nil || got["a"] != "First" || got["c"] != "" {
			t.Fatal("trailing blank originals must remain in the result", got, err)
		}
	}
}

func TestStringsXLSXLegacyHeaderAndLiteralHeaderText(t *testing.T) {
	expected := []StringRow{{"x", "Original X"}, {"y", "Original Y"}, {"z", "Original Z"}}
	legacy := badWorkbook(t, [][]any{{"id", "text"}, {"y", ""}, {}, {"x", " Перевод "}, {"z"}}, "strings")
	got, err := readStrings(legacy, expected)
	want := map[string]string{"x": " Перевод ", "y": "", "z": ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("legacy ID/text rows must retain their old empty and unordered semantics", got, err)
	}
	expected = []StringRow{{"real_id", "id"}}
	got, err = readStrings(badWorkbook(t, [][]any{{"id", "text"}}, "Тексты"), expected)
	if err != nil || got["real_id"] != "text" {
		t.Fatal("a real original 'id' and translation 'text' are not a technical header", got, err)
	}
	got, err = readStrings(badWorkbook(t, [][]any{{"id", "text"}, {"real_id", "translated"}}, "strings"), expected)
	if err != nil || got["real_id"] != "translated" {
		t.Fatal("legacy header must also work when the original happens to be 'id'", got, err)
	}
	got, err = readStrings(badWorkbook(t, [][]any{{"id", "text"}}, "strings"), nil)
	if err != nil || len(got) != 0 {
		t.Fatal("empty legacy workbook", got, err)
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
	for name, rows := range map[string][][]any{
		"empty":                   nil,
		"missing nonempty row":    {{"First"}},
		"extra row":               {{"First"}, {"Second"}, {"Extra"}},
		"changed original":        {{"Changed"}, {"Second"}},
		"reordered originals":     {{"Second"}, {"First"}},
		"blank nonempty original": {{"First"}, {""}},
		"third column":            {{"First", "translation", "extra"}, {"Second"}},
		"legacy wrong header":     {{"id", "wrong"}, {"x", "a"}, {"y", "b"}},
		"legacy duplicate ID":     {{"id", "text"}, {"x", "a"}, {"x", "b"}},
		"legacy missing ID":       {{"id", "text"}, {}, {"x", "a"}},
		"legacy empty ID":         {{"id", "text"}, {"", "a"}, {"y", "b"}},
		"legacy unknown ID":       {{"id", "text"}, {"unknown", "a"}, {"y", "b"}},
		"legacy third column":     {{"id", "text"}, {"x", "a", "extra"}, {"y", "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readStrings(badWorkbook(t, rows, "imported"), expected); err == nil {
				t.Fatal("accepted invalid text workbook")
			}
		})
	}
	if _, err := readStrings([]byte("not zip"), expected); err == nil {
		t.Fatal("accepted invalid XLSX bytes")
	}
	for _, legacy := range []bool{false, true} {
		for _, column := range []string{"A", "B", "C"} {
			t.Run(fmt.Sprint("formula ", legacy, column), func(t *testing.T) {
				rows, sheet, row := [][]any{{"First"}, {"Second"}}, "Тексты", "1"
				if legacy {
					rows, sheet, row = [][]any{{"id", "text"}, {"x", "a"}, {"y", "b"}}, "strings", "2"
				}
				raw := editStringsWorkbook(t, badWorkbook(t, rows, sheet), func(f *excelize.File) error { return f.SetCellFormula(sheet, column+row, "1+1") })
				if _, err := readStrings(raw, expected); err == nil || !strings.Contains(err.Error(), "cell") {
					t.Fatal("formula needs a cell-specific error", err)
				}
			})
		}
	}
	raw := badWorkbook(t, nil, "empty")
	raw = editStringsWorkbook(t, raw, func(f *excelize.File) error { f.WorkBook.Sheets.Sheet = nil; return nil })
	if _, err := readStrings(raw, expected); err == nil {
		t.Fatal("accepted workbook without a worksheet")
	}
}
