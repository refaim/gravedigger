package exe

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestStringsXLSX(t *testing.T) {
	want := []StringRow{{ID: "title", Text: "Dangerous Dave"}, {ID: "empty", Text: ""}, {ID: "unicode", Text: "Привет\nмир"}, {ID: "formula_text", Text: "=1+1"}}
	raw, err := writeStrings(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readStrings(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range want {
		if got[row.ID] != row.Text {
			t.Fatalf("%s: %q", row.ID, got[row.ID])
		}
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
	for _, raw := range [][]byte{
		[]byte("not zip"),
		badWorkbook(t, nil, "other"),
		badWorkbook(t, [][]any{{"wrong", "text"}}, "strings"),
		badWorkbook(t, [][]any{{"id", "wrong"}}, "strings"),
		badWorkbook(t, [][]any{{"id", "text"}, {"x", "a"}, {"x", "b"}}, "strings"),
		badWorkbook(t, [][]any{{"id", "text"}, {"", "text"}}, "strings"),
	} {
		if _, err := readStrings(raw); err == nil {
			t.Fatal("accepted invalid workbook")
		}
	}
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, err := range []error{f.SetSheetName("Sheet1", "strings"), f.SetCellStr("strings", "A1", "id"), f.SetCellStr("strings", "B1", "text"), f.SetCellStr("strings", "A2", "x"), f.SetCellFormula("strings", "B2", "1+1")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readStrings(b.Bytes()); err == nil {
		t.Fatal("accepted a formula as translated text")
	}
	raw, err := writeStrings([]StringRow{{ID: "x", Text: ""}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("PK")) {
		t.Fatal("not XLSX")
	}
}
