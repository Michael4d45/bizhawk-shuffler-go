//go:build !linux

package clienthost

// BizHawkLuaSocketInstalled is always true off Linux.
func BizHawkLuaSocketInstalled(string) bool {
	return true
}

// EnsureBizHawkLuaSocket is a no-op off Linux.
func EnsureBizHawkLuaSocket(string, func(string)) error {
	return nil
}

func bizHawkRootFromDataDir(string) (string, error) {
	return "", nil
}
