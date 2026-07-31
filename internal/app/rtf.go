package app
import (
	"errors"
	"strings"
)

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func rtfHexByte(h1, h2 byte) (byte, bool) {
	var v byte
	for _, c := range []byte{h1, h2} {
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 + c - '0'
		case c >= 'a' && c <= 'f':
			v = v<<4 + c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = v<<4 + c - 'A' + 10
		default:
			return 0, false
		}
	}
	return v, true
}

// rtfToPlainText снимает управляющие последовательности (достаточно для типичных книжных RTF).
func rtfToPlainText(s string) string {
	var b strings.Builder
	b.Grow(len(s) / 2)
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			if s[i] == '{' || s[i] == '}' {
				i++
				continue
			}
			if s[i] == '\r' {
				i++
				continue
			}
			b.WriteByte(s[i])
			i++
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		if s[i] == '\'' && i+2 < len(s) {
			if v, ok := rtfHexByte(s[i+1], s[i+2]); ok {
				b.WriteByte(v)
				i += 3
				continue
			}
		}
		if s[i] == '~' {
			b.WriteByte(' ')
			i++
			continue
		}
		if s[i] == '-' {
			i++
			continue
		}
		if s[i] == '\\' || s[i] == '{' || s[i] == '}' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if s[i] == '*' {
			i++
			continue
		}
		if s[i] == 'u' && i+1 < len(s) && (s[i+1] == '-' || (s[i+1] >= '0' && s[i+1] <= '9')) {
			j := i + 1
			sign := 1
			if s[j] == '-' {
				sign = -1
				j++
			}
			startDigits := j
			n := 0
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				n = n*10 + int(s[j]-'0')
				j++
			}
			if j > startDigits {
				r := rune(sign * n)
				if r != 0 && r != '\ufffd' {
					b.WriteRune(r)
				}
				if j < len(s) && s[j] != ' ' {
					j++
				}
				for j < len(s) && s[j] == ' ' {
					j++
				}
				i = j
				continue
			}
		}
		if !isASCIILetter(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && isASCIILetter(s[j]) {
			j++
		}
		word := s[i:j]
		i = j
		skipBin := 0
		if i < len(s) && s[i] == '-' {
			i++
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				skipBin = skipBin*10 + int(s[i]-'0')
				i++
			}
		} else if i < len(s) && s[i] >= '0' && s[i] <= '9' {
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				skipBin = skipBin*10 + int(s[i]-'0')
				i++
			}
		}
		if i < len(s) && s[i] == ' ' {
			i++
		}
		switch word {
		case "par", "line", "column", "sect", "page":
			b.WriteByte('\n')
		case "tab":
			b.WriteByte(' ')
		case "bin":
			if skipBin > 0 && skipBin < len(s) {
				if i+skipBin <= len(s) {
					i += skipBin
				}
			}
		default:
			// остальные команды отбрасываем
		}
	}
	return b.String()
}

func rtfToParagraphs(raw []byte) ([]string, error) {
	s := rtfToPlainText(string(raw))
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("rtf: пустой текст")
	}
	return paragraphsFromPlainText(s), nil
}
