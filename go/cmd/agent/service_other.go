//go:build !windows

package main

func maybeRunService(cfgPath string) error {
	return runInteractive(cfgPath)
}

func absolutizeConfigPath(cfgPath string) string {
	return cfgPath
}
