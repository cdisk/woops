package guac

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Encode builds one Guacamole instruction: len.value,len.value,...;
// Length prefixes are Unicode character (code point) counts, not UTF-8 bytes
// — required by the Guacamole protocol; using byte length breaks on CJK hostnames
// / passwords and desyncs the guacd handshake ("connect instruction was not received").
func Encode(opcode string, args ...string) []byte {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, elem(opcode))
	for _, a := range args {
		parts = append(parts, elem(a))
	}
	return []byte(strings.Join(parts, ",") + ";")
}

func elem(s string) string {
	return strconv.Itoa(utf8.RuneCountInString(s)) + "." + s
}

// ReadInstruction reads a single instruction from r.
func ReadInstruction(r *bufio.Reader) (opcode string, args []string, err error) {
	var parts []string
	for {
		lenStr, err := r.ReadString('.')
		if err != nil {
			return "", nil, err
		}
		if len(lenStr) < 2 || lenStr[len(lenStr)-1] != '.' {
			return "", nil, fmt.Errorf("guac: bad length prefix")
		}
		n, err := strconv.Atoi(lenStr[:len(lenStr)-1])
		if err != nil || n < 0 {
			return "", nil, fmt.Errorf("guac: invalid length")
		}
		val, err := readUTF8Chars(r, n)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, val)
		b, err := r.ReadByte()
		if err != nil {
			return "", nil, err
		}
		switch b {
		case ',':
			continue
		case ';':
			if len(parts) == 0 {
				return "", nil, fmt.Errorf("guac: empty instruction")
			}
			return parts[0], parts[1:], nil
		default:
			return "", nil, fmt.Errorf("guac: unexpected separator %q", b)
		}
	}
}

// readUTF8Chars reads n Unicode code points as UTF-8 from r.
func readUTF8Chars(r *bufio.Reader, n int) (string, error) {
	if n == 0 {
		return "", nil
	}
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		ch, _, err := r.ReadRune()
		if err != nil {
			return "", err
		}
		b.WriteRune(ch)
	}
	return b.String(), nil
}

// WriteInstruction writes one encoded instruction.
func WriteInstruction(w io.Writer, opcode string, args ...string) error {
	_, err := w.Write(Encode(opcode, args...))
	return err
}

// Instruction is one fully parsed Guacamole protocol instruction.
type Instruction struct {
	Opcode string
	Args   []string
}

func (i Instruction) Encode() []byte { return Encode(i.Opcode, i.Args...) }

// ParseInstructions parses one WebSocket message containing one or more
// complete Guacamole instructions. Partial/trailing data is rejected.
func ParseInstructions(data []byte) ([]Instruction, error) {
	raw := bytes.NewReader(data)
	br := bufio.NewReader(raw)
	var out []Instruction
	for raw.Len() > 0 || br.Buffered() > 0 {
		op, args, err := ReadInstruction(br)
		if err != nil {
			return nil, err
		}
		out = append(out, Instruction{Opcode: op, Args: args})
	}
	return out, nil
}
