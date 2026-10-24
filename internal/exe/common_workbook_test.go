package exe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestCommonWorkbookExport(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			original, editable := unpackFixture(t, variant)
			book, err := excelize.OpenReader(bytes.NewReader(editable.Translatable["strings.xlsx"]))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = book.Close() }()
			if names := book.GetSheetList(); len(names) != 2 || names[0] != "Тексты" || names[1] != "Шрифт" {
				t.Fatal("one common workbook needs consistently named text and font sheets", names)
			}
			var metadata exeMetadata
			if err := json.Unmarshal(editable.Manifest.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			data, start, err := decodeOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			_, originals, err := inspectStrings(data[start:])
			if err != nil {
				t.Fatal(err)
			}
			rows, err := book.GetRows("Тексты", excelize.Options{RawCellValue: true})
			if err != nil || len(rows) != len(metadata.Strings) {
				t.Fatal("text rows must contain only original/translation pairs, without an ID header", len(rows), err)
			}
			for i, entry := range metadata.Strings {
				originalText, err := book.GetCellValue("Тексты", fmt.Sprintf("A%d", i+1))
				if err != nil || originalText != strings.TrimRight(originals[entry.ID], "\n") {
					t.Fatal("original does not match the immutable metadata", entry.ID, originalText, err)
				}
				translation, err := book.GetCellValue("Тексты", fmt.Sprintf("B%d", i+1))
				if err != nil || translation != "" {
					t.Fatal("new projects must have an empty translation column", entry.ID, translation, err)
				}
			}
			out, err := (ExeTextHandler{}).Repack(editable.Manifest, editable.Translatable, editable.Meta)
			if err != nil || !bytes.Equal(out, original) {
				t.Fatal("blank translations must retain the exact original game", err)
			}
		})
	}
}

func TestCommonWorkbookDuplicateOriginalsAndRenamedSheet(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			_, editable := unpackFixture(t, variant)
			var metadata exeMetadata
			if err := json.Unmarshal(editable.Manifest.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			translations := map[string]string{
				"win_congrats":       "FIRST CONGRATULATIONS TRANSLATION!",
				"game_over_congrats": "SECOND CONGRATULATIONS TRANSLATION!",
			}
			atlasWorkbook(t, &editable, func(book *excelize.File) {
				var firstOriginal string
				for i, entry := range metadata.Strings {
					if translation, ok := translations[entry.ID]; ok {
						cell, _ := excelize.CoordinatesToCellName(1, i+1)
						text, err := book.GetCellValue("Тексты", cell)
						if err != nil {
							t.Fatal(err)
						}
						if firstOriginal != "" && firstOriginal != text {
							t.Fatal("fixture must really contain duplicate originals")
						}
						firstOriginal = text
						cell, _ = excelize.CoordinatesToCellName(2, i+1)
						if err := book.SetCellStr("Тексты", cell, translation); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := book.SetSheetName("Тексты", "Лист1"); err != nil {
					t.Fatal(err)
				}
			})
			out, err := (ExeTextHandler{}).Repack(editable.Manifest, editable.Translatable, editable.Meta)
			if err != nil {
				t.Fatal(err)
			}
			module := code(out)
			for id, pointer := range map[string]int{"win_congrats": 0x10fa, "game_over_congrats": 0x2430} {
				at := dsBase + word(module, pointer)
				end := bytes.IndexByte(module[at:], 0)
				if end < 0 || strings.TrimRight(string(module[at:at+end]), "\n") != translations[id] {
					t.Fatal("duplicate original was assigned to the wrong native caller", id)
				}
			}
			if word(module, 0xf28) != 1024 {
				t.Fatal("shared workbook relocation changed the level clear count")
			}
		})
	}
}

func TestCommonWorkbookRejectsChangedOriginals(t *testing.T) {
	for _, operation := range []string{"changed", "missing", "reordered", "extra", "formula", "missing font"} {
		t.Run(operation, func(t *testing.T) {
			_, editable := unpackFixture(t, "softdisk")
			atlasWorkbook(t, &editable, func(book *excelize.File) {
				var err error
				switch operation {
				case "changed":
					err = book.SetCellStr("Тексты", "A1", "Edited original")
				case "missing":
					err = book.RemoveRow("Тексты", 1)
				case "reordered":
					first, _ := book.GetCellValue("Тексты", "A1")
					second, _ := book.GetCellValue("Тексты", "A2")
					if err = book.SetCellStr("Тексты", "A1", second); err == nil {
						err = book.SetCellStr("Тексты", "A2", first)
					}
				case "extra":
					err = book.SetCellStr("Тексты", "A1000", "Extra source")
				case "formula":
					err = book.SetCellFormula("Тексты", "B1", "1+1")
				case "missing font":
					err = book.DeleteSheet("Шрифт")
				}
				if err != nil {
					t.Fatal(err)
				}
			})
			if _, err := (ExeTextHandler{}).Repack(editable.Manifest, editable.Translatable, editable.Meta); err == nil {
				t.Fatal("accepted an invalid common workbook", operation)
			}
		})
	}
}
