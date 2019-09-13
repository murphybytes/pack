package logging

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
)

func TestWithPrefixLogger(t *testing.T) {
	prevColor := color.NoColor
	color.NoColor = false
	defer func() {
		color.NoColor = prevColor
	}()

	tt := []struct {
		name      string
		prefix    string
		writes    []string
		want      string
		wantColor bool
	}{
		{
			name:   "no color",
			prefix: "foo--",
			writes: []string{
				"line one",
				" line two\n",
			},
			want:      "[foo--] line one line two\n",
			wantColor: false,
		},
		{
			name:   "with color",
			prefix: "foo--",
			writes: []string{
				color.HiBlueString("line one"),
				" line two\n",
			},
			want:      "[\x1b[36mfoo--\x1b[0m] \x1b[94mline one\x1b[0m line two\n",
			wantColor: true,
		},
		{
			name:   "don't want color",
			prefix: "foo--",
			writes: []string{
				color.HiBlueString("line one"),
				" line two\n",
			},
			want:      "[foo--] line one line two\n",
			wantColor: false,
		},
		{
			name:   "color partial buffer",
			prefix: "foo--",
			writes: []string{
				color.HiBlueString("line one"),
				" line two\n",
				color.GreenString("partial buffer []"),
			},
			want:      "[\x1b[36mfoo--\x1b[0m] \x1b[94mline one\x1b[0m line two\n[\x1b[36mfoo--\x1b[0m] \x1b[32mpartial buffer []\x1b[0m\n",
			wantColor: true,
		},
		{
			name:   "partial buffer no color",
			prefix: "build",
			writes: []string{
				color.RedString("part line "),
				color.BlueString("line end\n"),
				color.YellowString("end part"),
			},
			want:      "[build] part line line end\nend part\n",
			wantColor: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var buff bytes.Buffer
			wtr := NewPrefixWriter(&buff, tc.wantColor, tc.prefix)
			for _, line := range tc.writes {
				n, _ := wtr.Write([]byte(line))
				if n != len(line) {
					t.Fatalf("line length got %d want %d", n, len(line))
				}
			}
			wtr.Close()
			got := buff.String()
			if got != tc.want {
				t.Logf("want %q", tc.want)
				t.Logf("got  %q", got)
				t.Fatal("output string mismatch")
			}
		})
	}

}
