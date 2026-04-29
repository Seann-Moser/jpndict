package mdict

import (
	"fmt"
	"os"
	"sort"
)

type MDX struct {
	f      *os.File
	header *Header

	keyIndex []KeyIndexEntry

	recordBlocks []recordBlockInfo
	recordBase   int64
}

type recordBlockInfo struct {
	CompressedSize   uint64
	DecompressedSize uint64
	CompOffset       int64
	DecompOffset     uint64
}

func OpenMDX(path string) (*MDX, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	h, err := readHeader(f)
	if err != nil {
		return nil, err
	}

	m := &MDX{
		f:      f,
		header: h,
	}

	// TODO: parse key index next
	if err := m.readKeyIndex(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *MDX) Close() error {
	return m.f.Close()
}

func (m *MDX) Lookup(key string) ([]byte, error) {
	i := sort.Search(len(m.keyIndex), func(i int) bool {
		return m.keyIndex[i].Key >= key
	})

	if i < len(m.keyIndex) && m.keyIndex[i].Key == key {
		return m.readRecord(m.keyIndex[i])
	}

	return nil, fmt.Errorf("not found: %s", key)
}

func (m *MDX) HasKey(key string) bool {
	i := sort.Search(len(m.keyIndex), func(i int) bool {
		return m.keyIndex[i].Key >= key
	})

	return i < len(m.keyIndex) && m.keyIndex[i].Key == key
}
