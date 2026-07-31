// Huff/CDIC — порт calibre-9.8.0/src/calibre/ebooks/mobi/huffcdic.py (GPL-3.0).
// Используется для MOBI/AZW3 с compression type «DH» (0x4448).

package app
import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

type mobiActiveHeader struct {
	Compression   uint16
	Raw           []byte // первый рекорд MOBI или рекорд KF8 после BOUNDARY
	TextStart     int    // индекс Palm-рекорда первого текстового блока
	HuffGlobalOff int    // huff_offset из заголовка + база KF8
	HuffNumber    int
	Records       uint16
	ExtraFlags    uint16
	Codepage      uint32
}

func readU32BE(b []byte, o int) uint32 {
	if o+4 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint32(b[o : o+4])
}

func readU16BE(b []byte, o int) uint16 {
	if o+2 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint16(b[o : o+2])
}

// exthKF8HeaderRecord — EXTH запись 121: индекс Palm-рекорда с заголовком KF8.
func exthKF8HeaderRecord(rec0 []byte) (uint32, bool) {
	if len(rec0) < 0x84 {
		return 0, false
	}
	hdrLen := int(readU32BE(rec0, 20))
	exthFlag := readU32BE(rec0, 0x80)
	if exthFlag&0x40 == 0 {
		return 0, false
	}
	start := 16 + hdrLen
	if start+12 > len(rec0) {
		return 0, false
	}
	if string(rec0[start:start+4]) != "EXTH" {
		return 0, false
	}
	exthLen := int(readU32BE(rec0, start+4))
	numItems := int(readU32BE(rec0, start+8))
	pos := start + 12
	end := start + exthLen
	if end > len(rec0) {
		end = len(rec0)
	}
	for i := 0; i < numItems && pos+8 <= end; i++ {
		idx := readU32BE(rec0, pos)
		sz := int(readU32BE(rec0, pos+4))
		pos += 8
		if sz < 0 || pos+sz > end {
			break
		}
		content := rec0[pos : pos+sz]
		pos += sz
		if idx == 121 && len(content) >= 4 {
			return readU32BE(content, 0), true
		}
	}
	return 0, false
}

func mobiTrailingSize(data []byte, extraFlags uint16) int {
	if extraFlags == 0 || len(data) == 0 {
		return 0
	}
	sizeofTrailingEntry := func(ptr []byte, psize int) int {
		bitpos, result := 0, 0
		for psize > 0 {
			v := int(ptr[psize-1])
			result |= (v & 0x7F) << bitpos
			bitpos += 7
			psize--
			if (v&0x80) != 0 || bitpos >= 28 || psize == 0 {
				return result
			}
		}
		return result
	}
	num := 0
	size := len(data)
	flags := extraFlags >> 1
	for flags != 0 {
		if flags&1 != 0 {
			if size-num <= 0 {
				return 0
			}
			num += sizeofTrailingEntry(data, size-num)
		}
		flags >>= 1
	}
	if extraFlags&1 != 0 {
		off := size - num - 1
		if off >= 0 && off < len(data) {
			num += (int(data[off]) & 0x3) + 1
		}
	}
	return num
}

func resolveMobiActiveHeader(recs [][]byte) (*mobiActiveHeader, error) {
	if len(recs) < 2 {
		return nil, errors.New("mobi: мало записей")
	}
	rec0 := recs[0]
	kf8i, hasKF8 := exthKF8HeaderRecord(rec0)
	hdr := rec0
	textStart := 1
	huffBase := 0
	if hasKF8 && int(kf8i) > 0 && int(kf8i) < len(recs) {
		prev := bytes.TrimRight(recs[kf8i-1], "\x00")
		if bytes.Equal(prev, []byte("BOUNDARY")) {
			hdr = recs[int(kf8i)]
			textStart = int(kf8i) + 1
			huffBase = int(kf8i)
		}
	}
	if len(hdr) < 12 {
		return nil, errors.New("mobi: короткий заголовок")
	}
	comp := readU16BE(hdr, 0)
	records := readU16BE(hdr, 8)
	var extra uint16
	if len(hdr) >= 0xF4 {
		extra = readU16BE(hdr, 0xF2)
	}
	codepage := uint32(1252)
	if len(hdr) >= 28 {
		codepage = readU32BE(hdr, 24)
	}
	huffNum := 0
	huffOff := 0
	if comp == mobiHuffCDIC && len(hdr) >= 0x78 {
		huffOff = int(readU32BE(hdr, 0x70))
		huffNum = int(readU32BE(hdr, 0x74))
	}
	return &mobiActiveHeader{
		Compression:   comp,
		Raw:             hdr,
		TextStart:       textStart,
		HuffGlobalOff:   huffBase + huffOff,
		HuffNumber:      huffNum,
		Records:         records,
		ExtraFlags:      extra,
		Codepage:        codepage,
	}, nil
}

// --- Huff / CDIC unpacker (huffcdic.Reader) ---

type huffUnpacker struct {
	dict1      []struct{ codeLen int; term bool; maxCode uint32 }
	minCode    []uint32
	maxCode    []uint32
	dictionary []struct {
		raw      []byte
		flag     bool
		inflated []byte
	}
}

func dict1Unpack(v uint32) (codeLen int, term bool, maxCode uint32) {
	codeLen = int(v & 0x1f)
	term = v&0x80 != 0
	maxCode = v >> 8
	if codeLen == 0 {
		return 0, false, 0
	}
	if codeLen <= 8 {
		// assert term in Python
	}
	maxCode = ((maxCode + 1) << (32 - uint(codeLen))) - 1
	return codeLen, term, maxCode
}

func newHuffUnpacker(huffs [][]byte) (*huffUnpacker, error) {
	if len(huffs) == 0 {
		return nil, errors.New("huff: нет записей")
	}
	huff := huffs[0]
	if len(huff) < 16 || string(huff[0:8]) != "HUFF\x00\x00\x00\x18" {
		return nil, errors.New("huff: неверный заголовок HUFF")
	}
	off1 := int(readU32BE(huff, 8))
	off2 := int(readU32BE(huff, 12))
	if off1+256*4 > len(huff) || off2+64*4 > len(huff) {
		return nil, errors.New("huff: обрезан словарь")
	}
	u := &huffUnpacker{}
	u.dict1 = make([]struct {
		codeLen int
		term    bool
		maxCode uint32
	}, 256)
	for i := 0; i < 256; i++ {
		v := readU32BE(huff, off1+i*4)
		cl, tm, mc := dict1Unpack(v)
		u.dict1[i].codeLen = cl
		u.dict1[i].term = tm
		u.dict1[i].maxCode = mc
	}
	dict2 := make([]uint32, 64)
	for i := 0; i < 64; i++ {
		dict2[i] = readU32BE(huff, off2+i*4)
	}
	// Как в Python: (0,) + dict2[0::2] и (0,) + dict2[1::2], enumerate по codelen 0..32
	u.minCode = append(u.minCode, 0)
	for codelen := 1; codelen <= 32; codelen++ {
		mincode := dict2[(codelen-1)*2]
		u.minCode = append(u.minCode, mincode<<(32-uint(codelen)))
	}
	u.maxCode = append(u.maxCode, 0)
	for codelen := 1; codelen <= 32; codelen++ {
		maxcode := dict2[(codelen-1)*2+1]
		u.maxCode = append(u.maxCode, ((maxcode+1)<<(32-uint(codelen)))-1)
	}
	for _, cdic := range huffs[1:] {
		if err := u.loadCDIC(cdic); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (u *huffUnpacker) loadCDIC(cdic []byte) error {
	if len(cdic) < 16 || string(cdic[0:8]) != "CDIC\x00\x00\x00\x10" {
		return errors.New("huff: неверный CDIC")
	}
	phrases := readU32BE(cdic, 8)
	bits := readU32BE(cdic, 12)
	n := int(uint32(1) << bits)
	if n > int(phrases)-len(u.dictionary) {
		n = int(phrases) - len(u.dictionary)
	}
	if n <= 0 {
		return nil
	}
	if 16+n*2 > len(cdic) {
		return errors.New("huff: обрезан CDIC")
	}
	offs := make([]uint16, n)
	for i := 0; i < n; i++ {
		offs[i] = readU16BE(cdic, 16+i*2)
	}
	for _, off := range offs {
		base := 16 + int(off)
		if base+2 > len(cdic) {
			continue
		}
		blen := readU16BE(cdic, base)
		slice := cdic[base+2 : base+2+int(blen&0x7fff)]
		u.dictionary = append(u.dictionary, struct {
			raw      []byte
			flag     bool
			inflated []byte
		}{raw: append([]byte(nil), slice...), flag: blen&0x8000 != 0})
	}
	return nil
}

func readU64BE(b []byte, off int) uint64 {
	if off+8 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint64(b[off:])
}

func (u *huffUnpacker) unpack(data []byte) ([]byte, error) {
	bitsLeft := len(data) * 8
	data = append(append([]byte(nil), data...), make([]byte, 8)...)
	pos := 0
	x := readU64BE(data, pos)
	n := 32
	var parts [][]byte
	for {
		if n <= 0 {
			pos += 4
			if pos+8 > len(data) {
				break
			}
			x = readU64BE(data, pos)
			n += 32
		}
		code := uint32((x >> uint(n)) & 0xffffffff)
		if int(code>>24) >= len(u.dict1) {
			return nil, fmt.Errorf("huff: код вне dict1")
		}
		d1 := u.dict1[code>>24]
		codelen := d1.codeLen
		term := d1.term
		maxCode := d1.maxCode
		if !term {
			for codelen < len(u.minCode) && code < u.minCode[codelen] {
				codelen++
			}
			if codelen >= len(u.maxCode) {
				codelen = len(u.maxCode) - 1
			}
			maxCode = u.maxCode[codelen]
		}
		n -= codelen
		bitsLeft -= codelen
		if bitsLeft < 0 {
			break
		}
		r := int((maxCode - code) >> (32 - uint(codelen)))
		if r < 0 || r >= len(u.dictionary) {
			return nil, fmt.Errorf("huff: индекс словаря %d", r)
		}
		ph := &u.dictionary[r]
		var slice []byte
		if ph.flag {
			slice = ph.raw
		} else {
			if ph.inflated == nil {
				sub, err := u.unpack(ph.raw)
				if err != nil {
					return nil, err
				}
				ph.inflated = sub
			}
			slice = ph.inflated
		}
		parts = append(parts, slice)
	}
	return bytes.Join(parts, nil), nil
}

func mobiUnpackHuffCDIC(recs [][]byte, ah *mobiActiveHeader) ([]byte, error) {
	if ah.HuffNumber <= 0 || ah.HuffGlobalOff < 0 {
		return nil, errors.New("mobi: некорректные Huff-поля")
	}
	end := ah.HuffGlobalOff + ah.HuffNumber
	if end > len(recs) {
		return nil, errors.New("mobi: Huff-записи вне файла")
	}
	huffs := make([][]byte, 0, ah.HuffNumber)
	for i := ah.HuffGlobalOff; i < end; i++ {
		huffs = append(huffs, recs[i])
	}
	hr, err := newHuffUnpacker(huffs)
	if err != nil {
		return nil, err
	}
	textEnd := ah.TextStart + int(ah.Records)
	if textEnd > len(recs) {
		textEnd = len(recs)
	}
	var blob []byte
	for i := ah.TextStart; i < textEnd; i++ {
		sec := recs[i]
		tail := mobiTrailingSize(sec, ah.ExtraFlags)
		if tail > len(sec) {
			tail = 0
		}
		sec = sec[: len(sec)-tail]
		part, err := hr.unpack(sec)
		if err != nil {
			return nil, err
		}
		blob = append(blob, part...)
	}
	if len(blob) > 0 && blob[len(blob)-1] == '#' {
		blob = blob[:len(blob)-1]
	}
	blob = bytes.ReplaceAll(blob, []byte{0}, nil)
	return blob, nil
}
