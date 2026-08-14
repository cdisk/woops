//go:build windows

package filemanager

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func listRoots() ([]entry, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("GetLogicalDrives: %w", err)
	}
	out := make([]entry, 0, 8)
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A'+i)) + ":"
		out = append(out, entry{
			Name:  letter,
			Path:  letter + `\`,
			IsDir: true,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no logical drives found")
	}
	return out, nil
}
