//go:build !windows

package install

func requireElevated() error { return nil }

func ensureWinpty(p Paths, gateway, code, pin string, live bool) error {
	_, _, _, _, _ = p, gateway, code, pin, live
	return nil
}

func swapWinptyAfterStop(p Paths) error {
	_ = p
	return nil
}
