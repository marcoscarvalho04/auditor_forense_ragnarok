//go:build windows

package admin

import "golang.org/x/sys/windows"

// IsElevated verifica se o processo atual foi iniciado com token elevado (Administrador).
// A verificação usa a API nativa do Windows via OpenCurrentProcessToken.
func IsElevated() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}
