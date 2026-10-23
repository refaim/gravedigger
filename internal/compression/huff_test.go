package compression

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func simpleTree() []byte {
	tree := make([]byte, 1020)
	put16(tree, 1016, 0)
	put16(tree, 1018, 1)
	return tree
}

func huffFile(size uint32, tree, stream []byte) []byte {
	out := make([]byte, 8)
	copy(out, "HUFF")
	binary.LittleEndian.PutUint32(out[4:], size)
	out = append(out, tree...)
	return append(out, stream...)
}

func TestRealHuffman(t *testing.T) {
	for _, variant := range []string{"softdisk", "retail"} {
		for _, name := range []string{"TITLE1.DD2", "TITLE2.DD2", "PROGPIC.DD2", "STARPIC.DD2", "S_DAVE.DD2", "S_CHUNK1.DD2", "S_CHUNK2.DD2", "S_FRANK.DD2", "S_MASTER.DD2"} {
			t.Run(variant+"/"+name, func(t *testing.T) {
				input := corpus(t, variant, name)
				decoded, tree, err := HuffDecompress(input)
				if err != nil {
					t.Fatal(err)
				}
				if len(decoded) != int(binary.LittleEndian.Uint32(input[4:])) {
					t.Fatal("wrong decoded size")
				}
				output, err := HuffCompress(decoded, tree)
				if err != nil || !bytes.Equal(output, input) {
					t.Fatalf("byte exact roundtrip: %v", err)
				}
				decoded[10] ^= 1
				decoded[100] ^= 3
				output, err = HuffCompress(decoded, tree)
				if err != nil {
					t.Fatal(err)
				}
				got, _, err := HuffDecompress(output)
				if err != nil || !bytes.Equal(got, decoded) {
					t.Fatalf("edited roundtrip: %v", err)
				}
			})
		}
	}
}

func TestHuffmanBitBoundaries(t *testing.T) {
	for _, data := range [][]byte{nil, {0}, {1, 0, 1, 1, 0, 0, 1, 0}, {0, 1, 0, 1, 0, 1, 0, 1, 1}} {
		for _, tail := range [][]byte{nil, {0xa8, 0x42, 0x65}} {
			tree := append(simpleTree(), tail...)
			compressed, err := HuffCompress(data, tree)
			if err != nil {
				t.Fatal(err)
			}
			got, preserved, err := HuffDecompress(compressed)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("data %v: %v", data, err)
			}
			again, err := HuffCompress(got, preserved)
			if err != nil || !bytes.Equal(again, compressed) {
				t.Fatalf("boundary roundtrip %v", err)
			}
		}
	}
}

func TestInvalidHuffman(t *testing.T) {
	for _, input := range [][]byte{nil, []byte("HUFF"), make([]byte, 1028), huffFile(1, simpleTree(), nil), huffFile(9, simpleTree(), []byte{0}), huffFile(0xffffffff, simpleTree(), []byte{0})} {
		if _, _, err := HuffDecompress(input); err == nil {
			t.Fatal("accepted malformed Huffman data")
		}
	}
	if _, err := HuffCompress(nil, nil); err == nil {
		t.Fatal("accepted missing tree")
	}
	if _, err := HuffCompress([]byte{2}, simpleTree()); err == nil {
		t.Fatal("accepted unencodable byte")
	}
	for _, value := range []int{511, 510, 509} {
		tree := simpleTree()
		put16(tree, 1016, value)
		if value == 509 {
			put16(tree, 1012, 510)
		}
		if _, _, err := HuffDecompress(huffFile(1, tree, []byte{0})); err == nil {
			t.Fatal("accepted cyclic/out-of-range tree")
		}
		if _, err := HuffCompress([]byte{0}, tree); err == nil {
			t.Fatal("accepted cyclic/out-of-range tree")
		}
	}
}
