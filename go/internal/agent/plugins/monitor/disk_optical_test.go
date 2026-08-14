package monitor

import (
	"strings"
	"testing"
)

func TestSkipOpticalFilesystemTypes(t *testing.T) {
	for _, fs := range []string{"iso9660", "ISO9660", "udf", "UDF", "cdfs", "CDFS"} {
		if !skipFS[strings.ToLower(fs)] {
			t.Fatalf("expected skipFS to exclude %q", fs)
		}
	}
	if skipFS["ntfs"] || skipFS["ext4"] || skipFS["xfs"] {
		t.Fatal("real disks must not be skipped by optical filter")
	}
}
