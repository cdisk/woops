package filemanager

import "testing"

func TestSortEntriesDirsFirst(t *testing.T) {
	in := []entry{
		{Name: "z.txt", IsDir: false},
		{Name: "B", IsDir: true},
		{Name: "a.txt", IsDir: false},
		{Name: "A", IsDir: true},
	}
	sortEntries(in)
	want := []string{"A", "B", "a.txt", "z.txt"}
	for i, name := range want {
		if in[i].Name != name {
			t.Fatalf("index %d: got %q want %q (full=%v)", i, in[i].Name, name, names(in))
		}
	}
	if !in[0].IsDir || !in[1].IsDir || in[2].IsDir || in[3].IsDir {
		t.Fatalf("dir/file grouping wrong: %+v", in)
	}
}

func names(entries []entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}
