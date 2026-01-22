package workflow

import "testing"

func TestGetEditor(t *testing.T) {
	cases := []struct {
		name   string
		editor string
		visual string
		want   string
	}{
		{name: "editor set", editor: "nano", visual: "vim", want: "nano"},
		{name: "visual set", editor: "", visual: "vim", want: "vim"},
		{name: "defaults to vi", editor: "", visual: "", want: "vi"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EDITOR", tc.editor)
			t.Setenv("VISUAL", tc.visual)

			if got := getEditor(); got != tc.want {
				t.Fatalf("getEditor() = %q, want %q", got, tc.want)
			}
		})
	}
}
