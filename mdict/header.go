package mdict

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

type Header struct {
	XMLName xml.Name   `xml:"Dictionary"`
	Attrs   []xml.Attr `xml:",any,attr"`
}

func (h *Header) Get(name string) string {
	for _, a := range h.Attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func readHeader(r io.Reader) (*Header, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, fmt.Errorf("read header size: %w", err)
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// MDX v2 usually has a 4-byte checksum after the header.
	// If your reader is an *os.File, this advances correctly.
	_, _ = io.CopyN(io.Discard, r, 4)

	xmlText := decodeHeaderXML(buf)

	var h Header
	if err := xml.Unmarshal([]byte(xmlText), &h); err != nil {
		return nil, fmt.Errorf("parse header XML: %w\nheader=%q", err, xmlText[:min(len(xmlText), 200)])
	}

	return &h, nil
}

func decodeHeaderXML(buf []byte) string {
	buf = bytes.TrimRight(buf, "\x00")

	// UTF-16LE pattern: <\x00D\x00i\x00c\x00...
	if len(buf) >= 4 && buf[0] == '<' && buf[1] == 0 {
		return decodeUTF16LE(buf)
	}

	// UTF-16BE pattern: \x00<\x00D\x00i...
	if len(buf) >= 4 && buf[0] == 0 && buf[1] == '<' {
		return decodeUTF16BE(buf)
	}

	return string(buf)
}

func decodeUTF16LE(buf []byte) string {
	u16 := make([]uint16, 0, len(buf)/2)

	for i := 0; i+1 < len(buf); i += 2 {
		u16 = append(u16, uint16(buf[i])|uint16(buf[i+1])<<8)
	}

	return strings.TrimRight(string(utf16.Decode(u16)), "\x00")
}

func decodeUTF16BE(buf []byte) string {
	u16 := make([]uint16, 0, len(buf)/2)

	for i := 0; i+1 < len(buf); i += 2 {
		u16 = append(u16, uint16(buf[i])<<8|uint16(buf[i+1]))
	}

	return strings.TrimRight(string(utf16.Decode(u16)), "\x00")
}
