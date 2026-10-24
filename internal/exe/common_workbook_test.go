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
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 || len(rows[0]) != 3 || rows[0][0] != "ID" || rows[0][1] != "Английский" || rows[0][2] != "Русский" {
				t.Fatal("text sheet must begin with ID/Английский/Русский headers")
			}
			if len(rows) != len(metadata.Strings)+1 {
				t.Fatal("text sheet must contain one header and every string", len(rows))
			}
			for i, entry := range metadata.Strings {
				id, err := book.GetCellValue("Тексты", fmt.Sprintf("A%d", i+2))
				if err != nil || id != entry.ID {
					t.Fatal("ID does not match the immutable metadata", entry.ID, id, err)
				}
				originalText, err := book.GetCellValue("Тексты", fmt.Sprintf("B%d", i+2))
				if err != nil || originalText != strings.TrimRight(originals[entry.ID], "\n") {
					t.Fatal("original does not match the immutable metadata", entry.ID, originalText, err)
				}
				translation, err := book.GetCellValue("Тексты", fmt.Sprintf("C%d", i+2))
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
				var translatedRows []int
				for i, entry := range metadata.Strings {
					if translation, ok := translations[entry.ID]; ok {
						cell, _ := excelize.CoordinatesToCellName(2, i+2)
						text, err := book.GetCellValue("Тексты", cell)
						if err != nil {
							t.Fatal(err)
						}
						if firstOriginal != "" && firstOriginal != text {
							t.Fatal("fixture must really contain duplicate originals")
						}
						firstOriginal = text
						cell, _ = excelize.CoordinatesToCellName(3, i+2)
						if err := book.SetCellStr("Тексты", cell, translation); err != nil {
							t.Fatal(err)
						}
						translatedRows = append(translatedRows, i+2)
					}
				}
				if len(translatedRows) != 2 {
					t.Fatal("fixture must contain both independently translated IDs")
				}
				// Moving complete rows must retain the association with each ID,
				// including two rows whose original text is identical.
				for column := 1; column <= 3; column++ {
					first, _ := excelize.CoordinatesToCellName(column, translatedRows[0])
					second, _ := excelize.CoordinatesToCellName(column, translatedRows[1])
					firstValue, err := book.GetCellValue("Тексты", first)
					if err != nil {
						t.Fatal(err)
					}
					secondValue, err := book.GetCellValue("Тексты", second)
					if err != nil {
						t.Fatal(err)
					}
					if err := book.SetCellStr("Тексты", first, secondValue); err != nil {
						t.Fatal(err)
					}
					if err := book.SetCellStr("Тексты", second, firstValue); err != nil {
						t.Fatal(err)
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
	for _, operation := range []string{"changed original", "changed ID", "missing", "extra", "translation formula", "original formula", "missing font"} {
		t.Run(operation, func(t *testing.T) {
			_, editable := unpackFixture(t, "softdisk")
			atlasWorkbook(t, &editable, func(book *excelize.File) {
				var err error
				switch operation {
				case "changed original":
					err = book.SetCellStr("Тексты", "B2", "Edited original")
				case "changed ID":
					err = book.SetCellStr("Тексты", "A2", "unknown_id")
				case "missing":
					err = book.RemoveRow("Тексты", 2)
				case "extra":
					err = book.SetCellStr("Тексты", "A1000", "Extra source")
				case "translation formula":
					err = book.SetCellFormula("Тексты", "C2", "1+1")
				case "original formula":
					err = book.SetCellFormula("Тексты", "B2", "1+1")
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

func TestCommonWorkbookRejectsOldTwoColumns(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		for _, format := range []string{"id/text", "original/translation"} {
			t.Run(variant+"/"+format, func(t *testing.T) {
				_, editable := unpackFixture(t, variant)
				atlasWorkbook(t, &editable, func(book *excelize.File) {
					// Only the text-sheet schema is old; keep every current PNG,
					// metadata entry and Unicode assignment valid and present.
					if format == "id/text" {
						if err := book.RemoveCol("Тексты", "C"); err != nil {
							t.Fatal(err)
						}
						if err := book.SetSheetRow("Тексты", "A1", &[]any{"id", "text"}); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := book.RemoveCol("Тексты", "A"); err != nil {
							t.Fatal(err)
						}
						if err := book.RemoveRow("Тексты", 1); err != nil {
							t.Fatal(err)
						}
					}
				})
				if _, err := (ExeTextHandler{}).Repack(editable.Manifest, editable.Translatable, editable.Meta); err == nil {
					t.Fatal("old two-column books require a fresh export", format)
				}
			})
		}
	}
}
