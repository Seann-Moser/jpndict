package mdict

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

type KeyIndexEntry struct {
	Key       string
	RecordPos int64
}

type keyBlockInfo struct {
	CompressedSize   uint64
	DecompressedSize uint64
}

func (m *MDX) readKeyIndex() error {
	var (
		numKeyBlocks          uint64
		numEntries            uint64
		keyBlockInfoDecompLen uint64
		keyBlockInfoCompLen   uint64
		keyBlocksTotalLen     uint64
	)

	for _, dst := range []*uint64{
		&numKeyBlocks,
		&numEntries,
		&keyBlockInfoDecompLen,
		&keyBlockInfoCompLen,
		&keyBlocksTotalLen,
	} {
		if err := binary.Read(m.f, binary.BigEndian, dst); err != nil {
			return fmt.Errorf("read key header number: %w", err)
		}
	}

	// v2 has a 4-byte adler32 after the 5 uint64 values.
	if _, err := io.CopyN(io.Discard, m.f, 4); err != nil {
		return fmt.Errorf("skip key header checksum: %w", err)
	}

	keyInfoCompressed := make([]byte, keyBlockInfoCompLen)
	if _, err := io.ReadFull(m.f, keyInfoCompressed); err != nil {
		return fmt.Errorf("read compressed key block info: %w", err)
	}

	keyInfo, err := decodeMDictBlock(keyInfoCompressed)
	if err != nil {
		return fmt.Errorf("decode key block info: %w", err)
	}

	if uint64(len(keyInfo)) != keyBlockInfoDecompLen {
		return fmt.Errorf(
			"key block info decompressed size mismatch: got=%d want=%d",
			len(keyInfo),
			keyBlockInfoDecompLen,
		)
	}

	utf16Keys := strings.EqualFold(m.header.Get("Encoding"), "UTF-16") ||
		strings.EqualFold(m.header.Get("Encoding"), "UTF-16LE")

	fmt.Printf("encoding=%q numKeyBlocks=%d keyInfoLen=%d\n",
		m.header.Get("Encoding"),
		numKeyBlocks,
		len(keyInfo),
	)
	infos, err := parseKeyBlockInfoV2(keyInfo, numKeyBlocks, utf16Keys)
	if err != nil {
		return err
	}

	keyBlocksCompressed := make([]byte, keyBlocksTotalLen)
	if _, err := io.ReadFull(m.f, keyBlocksCompressed); err != nil {
		return fmt.Errorf("read key blocks: %w", err)
	}

	var entries []KeyIndexEntry
	pos := 0

	for i, info := range infos {
		end := pos + int(info.CompressedSize)
		if end > len(keyBlocksCompressed) {
			return fmt.Errorf("key block %d exceeds buffer", i)
		}

		block, err := decodeMDictBlock(keyBlocksCompressed[pos:end])
		if err != nil {
			return fmt.Errorf("decode key block %d: %w", i, err)
		}

		if uint64(len(block)) != info.DecompressedSize {
			return fmt.Errorf(
				"key block %d decompressed size mismatch: got=%d want=%d",
				i,
				len(block),
				info.DecompressedSize,
			)
		}

		blockEntries, err := parseKeyBlockV2(block)
		if err != nil {
			return fmt.Errorf("parse key block %d: %w", i, err)
		}

		entries = append(entries, blockEntries...)
		pos = end
	}

	if uint64(len(entries)) != numEntries {
		return fmt.Errorf("entry count mismatch: got=%d want=%d", len(entries), numEntries)
	}

	m.keyIndex = entries

	// Important: after this point, m.f is positioned at record block info.
	// Save this later if you add it to MDX:
	// m.recordBlockOffset = current offset
	off, err := m.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("get record block offset: %w", err)
	}

	m.recordBase = off

	if err := m.readRecordIndex(); err != nil {
		return err
	}

	return nil
}

func (m *MDX) readRecordIndex() error {
	var numRecordBlocks uint64
	var numEntries uint64
	var recordBlockInfoSize uint64
	var recordBlockSize uint64

	for _, dst := range []*uint64{
		&numRecordBlocks,
		&numEntries,
		&recordBlockInfoSize,
		&recordBlockSize,
	} {
		if err := binary.Read(m.f, binary.BigEndian, dst); err != nil {
			return fmt.Errorf("read record header: %w", err)
		}
	}

	infoBuf := make([]byte, recordBlockInfoSize)
	if _, err := io.ReadFull(m.f, infoBuf); err != nil {
		return fmt.Errorf("read record block info: %w", err)
	}

	recordDataBase, err := m.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("get record data offset: %w", err)
	}

	r := bytes.NewReader(infoBuf)

	m.recordBlocks = make([]recordBlockInfo, 0, numRecordBlocks)

	var compOffset int64
	var decompOffset uint64

	for i := uint64(0); i < numRecordBlocks; i++ {
		var compSize uint64
		var decompSize uint64

		if err := binary.Read(r, binary.BigEndian, &compSize); err != nil {
			return fmt.Errorf("read record compressed size: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &decompSize); err != nil {
			return fmt.Errorf("read record decompressed size: %w", err)
		}

		m.recordBlocks = append(m.recordBlocks, recordBlockInfo{
			CompressedSize:   compSize,
			DecompressedSize: decompSize,
			CompOffset:       recordDataBase + compOffset,
			DecompOffset:     decompOffset,
		})

		compOffset += int64(compSize)
		decompOffset += decompSize
	}

	_ = recordBlockSize
	_ = numEntries

	return nil
}

func (m *MDX) readRecord(k KeyIndexEntry) ([]byte, error) {
	if len(m.keyIndex) == 0 {
		return nil, fmt.Errorf("empty key index")
	}

	start := uint64(k.RecordPos)

	next := uint64(0)
	found := false

	for i, entry := range m.keyIndex {
		if entry.Key == k.Key {
			if i+1 < len(m.keyIndex) {
				next = uint64(m.keyIndex[i+1].RecordPos)
			} else if len(m.recordBlocks) > 0 {
				last := m.recordBlocks[len(m.recordBlocks)-1]
				next = last.DecompOffset + last.DecompressedSize
			}
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("key not found in index: %s", k.Key)
	}

	block, err := m.recordBlockForOffset(start)
	if err != nil {
		return nil, err
	}

	raw, err := m.readRecordBlock(block)
	if err != nil {
		return nil, err
	}

	localStart := start - block.DecompOffset
	localEnd := next - block.DecompOffset

	if localStart > uint64(len(raw)) || localEnd > uint64(len(raw)) || localStart > localEnd {
		return nil, fmt.Errorf(
			"invalid record slice: start=%d end=%d blockLen=%d",
			localStart,
			localEnd,
			len(raw),
		)
	}

	return raw[localStart:localEnd], nil
}

func (m *MDX) recordBlockForOffset(offset uint64) (recordBlockInfo, error) {
	for _, b := range m.recordBlocks {
		start := b.DecompOffset
		end := b.DecompOffset + b.DecompressedSize

		if offset >= start && offset < end {
			return b, nil
		}
	}

	return recordBlockInfo{}, fmt.Errorf("record offset not found: %d", offset)
}

func (m *MDX) readRecordBlock(b recordBlockInfo) ([]byte, error) {
	buf := make([]byte, b.CompressedSize)

	if _, err := m.f.ReadAt(buf, b.CompOffset); err != nil {
		return nil, fmt.Errorf("read record block: %w", err)
	}

	out, err := decodeMDictBlock(buf)
	if err != nil {
		return nil, fmt.Errorf("decode record block: %w", err)
	}

	if uint64(len(out)) != b.DecompressedSize {
		return nil, fmt.Errorf(
			"record block decompressed size mismatch: got=%d want=%d",
			len(out),
			b.DecompressedSize,
		)
	}

	return out, nil
}
func parseKeyBlockInfoV2(buf []byte, numBlocks uint64, utf16Keys bool) ([]keyBlockInfo, error) {
	r := bytes.NewReader(buf)

	infos := make([]keyBlockInfo, 0, numBlocks)

	for i := uint64(0); i < numBlocks; i++ {
		var entriesInBlock uint64
		if err := binary.Read(r, binary.BigEndian, &entriesInBlock); err != nil {
			return nil, fmt.Errorf("read entries in key block: %w", err)
		}

		if err := skipSizedKeyText(r, utf16Keys); err != nil {
			return nil, fmt.Errorf("skip first key: %w", err)
		}

		if err := skipSizedKeyText(r, utf16Keys); err != nil {
			return nil, fmt.Errorf("skip last key: %w", err)
		}

		var compSize uint64
		var decompSize uint64

		if err := binary.Read(r, binary.BigEndian, &compSize); err != nil {
			return nil, fmt.Errorf("read compressed size: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &decompSize); err != nil {
			return nil, fmt.Errorf("read decompressed size: %w", err)
		}

		_ = entriesInBlock

		infos = append(infos, keyBlockInfo{
			CompressedSize:   compSize,
			DecompressedSize: decompSize,
		})
	}

	return infos, nil
}

func skipSizedKeyText(r *bytes.Reader, utf16Keys bool) error {
	var chars uint16
	if err := binary.Read(r, binary.BigEndian, &chars); err != nil {
		return err
	}

	width := int64(1)
	if utf16Keys {
		width = 2
	}

	// key chars + null terminator
	n := (int64(chars) + 1) * width

	if int64(r.Len()) < n {
		return fmt.Errorf(
			"not enough bytes for key text: chars=%d utf16=%v need=%d remaining=%d",
			chars,
			utf16Keys,
			n,
			r.Len(),
		)
	}

	_, err := io.CopyN(io.Discard, r, n)
	return err
}
func parseKeyBlockV2(buf []byte) ([]KeyIndexEntry, error) {
	r := bytes.NewReader(buf)

	var entries []KeyIndexEntry

	for r.Len() > 0 {
		var recordOffset uint64
		if err := binary.Read(r, binary.BigEndian, &recordOffset); err != nil {
			return nil, fmt.Errorf("read record offset: %w", err)
		}

		key, err := readNullTerminatedUTF8(r)
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}

		entries = append(entries, KeyIndexEntry{
			Key:       key,
			RecordPos: int64(recordOffset),
		})
	}

	return entries, nil
}

func decodeMDictBlock(buf []byte) ([]byte, error) {
	if len(buf) < 8 {
		return nil, fmt.Errorf("block too small")
	}

	blockType := binary.LittleEndian.Uint32(buf[:4])
	payload := buf[8:] // type + checksum

	switch blockType {
	case 0:
		return payload, nil

	case 2:
		zr, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		defer zr.Close()

		return io.ReadAll(zr)

	case 1:
		return nil, fmt.Errorf("lzo block not supported yet")

	default:
		return nil, fmt.Errorf("unknown mdict block type: %d", blockType)
	}
}

func readNullTerminatedUTF8(r *bytes.Reader) (string, error) {
	var b []byte

	for {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			break
		}
		b = append(b, c)
	}

	return strings.TrimSpace(string(b)), nil
}
