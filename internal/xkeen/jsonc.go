package xkeen

import (
	"encoding/json"
	"fmt"
	"os"
)

// StripJSONComments removes // and /* */ comments without touching string contents.
// Xray parses its own configs with a reader that tolerates comments, and XKeen does
// the same before jq (strip_json_comments in 04_register_init.sh), so real configs
// are full of them and plain json.Unmarshal chokes.
func StripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))

	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		c := data[i]

		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}

		if c == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				for i < len(data) && data[i] != '\n' {
					i++
				}
				// Keep the newline so parser error line numbers stay accurate
				if i < len(data) {
					out = append(out, '\n')
				}
				continue
			case '*':
				i += 2
				for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
					if data[i] == '\n' {
						out = append(out, '\n')
					}
					i++
				}
				i++
				continue
			}
		}

		out = append(out, c)
	}

	return out
}

// ReadJSONC reads a comment-tolerant JSON file into any structure.
func ReadJSONC(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ошибка чтения %s: %w", path, err)
	}

	if err := json.Unmarshal(StripJSONComments(data), v); err != nil {
		return fmt.Errorf("ошибка парсинга %s: %w", path, err)
	}

	return nil
}
