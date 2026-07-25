package cmd

import "testing"

func TestSanitizeTerminalField(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"plain path unchanged":     {"/blog/post", "/blog/post"},
		"ansi escape neutralized":  {"/a\x1b[31mred\x1b[0m", "/a�[31mred�[0m"},
		"carriage return replaced": {"/a\rHACKED", "/a�HACKED"},
		"newline replaced":         {"/a\nb", "/a�b"},
		"tab becomes space":        {"/a\tb", "/a b"},
		"del and null replaced":    {"/a\x7f\x00b", "/a��b"},
		"unicode printable kept":   {"/café", "/café"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := sanitizeTerminalField(c.in); got != c.want {
				t.Errorf("sanitizeTerminalField(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
