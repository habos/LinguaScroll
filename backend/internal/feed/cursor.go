package feed

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Cursor is the only state carried between pages: the channels of the last
// few items (for spacing across page boundaries) and the page number.
// Which clips were served lives in feed_seen.
type Cursor struct {
	V      int      `json:"v"`
	Page   int      `json:"p"`
	Recent []string `json:"r,omitempty"`
}

const cursorVersion = 1

// ErrBadCursor means the cursor wasn't one we issued.
var ErrBadCursor = errors.New("invalid cursor")

// Encode returns the opaque string form.
func (c Cursor) Encode() string {
	c.V = cursorVersion
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses s; an empty s is the first page.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil || c.V != cursorVersion || c.Page < 0 || len(c.Recent) > 50 {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}
