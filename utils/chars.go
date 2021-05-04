package utils

import (
	"strconv"
	"unicode/utf8"
)

type Char struct {
	id int32
}

func Code(r rune) *Char {
	return &Char{r}
}

func (c *Char) IsNull() bool {
	return c.id == 0
}

func (c *Char) IsWhitespace() bool {
	switch c.id {
	case '\t', '\v', '\f', '\r', ' ', 0x85, 0xA0: // use \n as LF
		return true
	}
	return false
}

func (c *Char) IsDigital() bool {
	return 47 < c.id && c.id < 58
}

func (c *Char) IsAlpha() bool {
	return (64 < c.id && c.id < 91) || (96 < c.id && c.id < 123) || c.id == 95
}

func (c *Char) Equal(s interface{}) bool {
	switch s.(type) {
	case int:
		return int(c.id) == s
	case string:
		if s != "" {
			r, _ := utf8.DecodeRune([]byte(s.(string)))
			return r == c.id
		}
		return c.id == 0
	}
	return false
}

func (c *Char) IsAlNum() bool {
	return c.IsDigital() || c.IsAlpha()
}

func (c *Char) Rune() int32 {
	return c.id
}

func (c *Char) Quote() string {
	return strconv.Quote(string(c.id))
}
