package exe

import (
	"bytes"
	"testing"
)

func TestDOSCP866Encoding(t *testing.T) {
	got, err := encodeDOSText("АЗЯ азя Ёё!\n")
	want := []byte{0x80, 0x87, 0x9f, ' ', 0xa0, 0xa7, 0xef, ' ', 0xf0, 0xf1, '!', '\n'}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("CP866 = %x, %v; want %x", got, err, want)
	}
	for _, text := range []string{"emoji🙂", "\x00", "\t", "\x7f"} {
		if _, err := encodeDOSText(text); err == nil {
			t.Fatalf("accepted unsupported DOS text %q", text)
		}
	}
}

func TestRussianDOSMessagesRoundtrip(t *testing.T) {
	h := ExeTextHandler{}
	for _, variant := range []string{"softdisk", "retail"} {
		t.Run(variant, func(t *testing.T) {
			_, u := unpackFixture(t, variant)
			setText(t, &u, "startup_executing", "ЗАПУСК ИГРЫ")
			out, err := h.Repack(u.Manifest, u.Translatable, u.Meta)
			if err != nil {
				t.Fatal(err)
			}
			again, err := h.Unpack("DAVE.EXE", out)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := readUnpackedStrings(t, again)
			if err != nil {
				t.Fatal(err)
			}
			if rows["startup_executing"] != "ЗАПУСК ИГРЫ" {
				t.Fatalf("decoded DOS text = %q", rows["startup_executing"])
			}
			same, err := h.Repack(again.Manifest, again.Translatable, again.Meta)
			if err != nil || !bytes.Equal(same, out) {
				t.Fatalf("Russian no-edit roundtrip changed bytes: %v", err)
			}
		})
	}
}

func TestAdditionalVisibleMessagesAreExported(t *testing.T) {
	_, u := unpackFixture(t, "softdisk")
	rows, err := readUnpackedStrings(t, u)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"debug_sprite_title":     "Sprite Test",
		"debug_sprite_label":     "Sprite:\nWidth:\nHeight:\nSource:",
		"memory_block_missing":   "Memory manager error: Block not found!",
		"memory_too_many_blocks": "Memory manager error: Too many blocks!",
		"memory_out_of_memory":   "Memory manager error: Out of memory!",
	} {
		if rows[id] != want {
			t.Errorf("%s = %q; want %q", id, rows[id], want)
		}
	}
	for _, id := range []string{"memory_block_missing", "memory_too_many_blocks", "memory_out_of_memory"} {
		if !isDOSString(id) {
			t.Errorf("%s must use the DOS font", id)
		}
	}
}
