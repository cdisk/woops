//go:build !windows

package install

func requireElevated() error { return nil }

func ensureWinpty(p Paths, gateway, code, pin string) error {
	_, _, _, _ = p, gateway, code, pin
	return nil
}
