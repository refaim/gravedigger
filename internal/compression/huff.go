package compression

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	huffTreeSize  = 1020
	huffRoot      = 254
	maxHuffOutput = 16 << 20 // Resources must fit bounded memory, even with corrupt sizes.
)

func huffCodes(tree []byte) ([256][]byte, error) {
	var codes [256][]byte
	if len(tree) < huffTreeSize {
		return codes, errors.New("truncated Huffman tree")
	}
	var seen [255]bool
	var walk func(int, []byte) error
	walk = func(node int, path []byte) error {
		if node < 256 {
			codes[node] = bytes.Clone(path)
			return nil
		}
		index := node - 256
		if index >= 255 || seen[index] {
			return errors.New("invalid or cyclic Huffman tree")
		}
		seen[index] = true
		for bit := 0; bit < 2; bit++ {
			next := append(bytes.Clone(path), byte(bit))
			if err := walk(u16(tree, index*4+bit*2), next); err != nil {
				return err
			}
		}
		return nil
	}
	err := walk(huffRoot+256, nil)
	return codes, err
}

// HuffDecompress returns the resource and its original tree plus unused tail.
func HuffDecompress(data []byte) ([]byte, []byte, error) {
	if len(data) < 8+huffTreeSize || !bytes.Equal(data[:4], []byte("HUFF")) {
		return nil, nil, errors.New("invalid HUFF header")
	}
	tree := data[8 : 8+huffTreeSize]
	if _, err := huffCodes(tree); err != nil {
		return nil, nil, err
	}
	size := int(binary.LittleEndian.Uint32(data[4:]))
	if size > maxHuffOutput {
		return nil, nil, errors.New("HUFF resource exceeds 16 MiB")
	}
	stream := data[8+huffTreeSize:]
	out := make([]byte, 0, size)
	position, node := 0, huffRoot
	for len(out) < size {
		if position/8 >= len(stream) {
			return nil, nil, errors.New("truncated HUFF bitstream")
		}
		bit := int(stream[position/8] >> uint(position%8) & 1)
		position++
		value := u16(tree, node*4+bit*2)
		if value >= 256 {
			node = value - 256
			continue
		}
		out = append(out, byte(value))
		node = huffRoot
	}
	preserved := append(bytes.Clone(tree), stream[position/8:]...)
	return out, preserved, nil
}

// HuffCompress preserves unused bits and trailing data for byte-exact roundtrips.
func HuffCompress(data, treeAndTail []byte) ([]byte, error) {
	codes, err := huffCodes(treeAndTail)
	if err != nil {
		return nil, err
	}
	if len(data) > maxHuffOutput {
		return nil, errors.New("HUFF resource exceeds 16 MiB")
	}
	out := make([]byte, 8, 8+huffTreeSize+len(data))
	copy(out, "HUFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(data)))
	out = append(out, treeAndTail[:huffTreeSize]...)
	var current byte
	position := uint(0)
	for _, value := range data {
		if len(codes[value]) == 0 {
			return nil, errors.New("byte absent from Huffman tree")
		}
		for _, bit := range codes[value] {
			current |= bit << position
			position++
			if position == 8 {
				out = append(out, current)
				current = 0
				position = 0
			}
		}
	}
	tail := treeAndTail[huffTreeSize:]
	if position != 0 {
		if len(tail) != 0 {
			current |= tail[0] &^ byte((1<<position)-1)
			tail = tail[1:]
		}
		out = append(out, current)
	}
	return append(out, tail...), nil
}
