//go:build !windows

package monitor

// isOpticalDrive is Windows-only (GetDriveType). On Unix, optical media is
// filtered via skipFS (iso9660 / udf / cdfs) once mounted.
func isOpticalDrive(mount string) bool {
	return false
}
