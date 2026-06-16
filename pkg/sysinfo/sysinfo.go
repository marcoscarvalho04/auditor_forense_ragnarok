//go:build windows

package sysinfo

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Collect escreve hostname, usuário, versão do Windows e timestamp NTP em system_info.txt.
func Collect(auditDir string) error {
	var sb strings.Builder

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "(erro: " + err.Error() + ")"
	}
	sb.WriteString("=== METADADOS DO SISTEMA ===\n\n")
	sb.WriteString("Hostname       : " + hostname + "\n")
	sb.WriteString("Usuário        : " + os.Getenv("USERNAME") + "\n")
	sb.WriteString("Domínio        : " + os.Getenv("USERDOMAIN") + "\n")

	winVer, build, err := windowsVersion()
	if err != nil {
		winVer = "(erro ao ler registro)"
	}
	sb.WriteString("Windows        : " + winVer + "\n")
	sb.WriteString("Build          : " + build + "\n")

	// Tenta NTP primário, com fallback.
	ntpTime, ntpSrc, err := queryNTPWithFallback()
	if err != nil {
		sb.WriteString("Timestamp NTP  : (erro: " + err.Error() + ")\n")
	} else {
		sb.WriteString("Timestamp NTP  : " + ntpTime.UTC().Format(time.RFC3339) + " (fonte: " + ntpSrc + ")\n")
	}
	sb.WriteString("Timestamp Local: " + time.Now().Format(time.RFC3339) + "\n")

	outPath := filepath.Join(auditDir, "system_info.txt")
	return os.WriteFile(outPath, []byte(sb.String()), 0644)
}

// windowsVersion lê ProductName e CurrentBuild do registro do Windows NT.
func windowsVersion() (product, build string, err error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", err
	}
	defer k.Close()

	product, _, err = k.GetStringValue("ProductName")
	if err != nil {
		product = "Windows (desconhecido)"
	}
	// DisplayVersion existe no Windows 10 20H2+; CurrentVersion é o fallback.
	displayVer, _, _ := k.GetStringValue("DisplayVersion")
	if displayVer != "" {
		product += " " + displayVer
	}
	build, _, err = k.GetStringValue("CurrentBuild")
	if err != nil {
		build = "?"
	}
	ubr, _, _ := k.GetIntegerValue("UBR") // Update Build Revision
	if ubr > 0 {
		build = fmt.Sprintf("%s.%d", build, ubr)
	}
	return product, build, nil
}

// queryNTPWithFallback tenta servidores NTP em ordem até obter resposta.
func queryNTPWithFallback() (t time.Time, source string, err error) {
	servers := []string{
		"a.st1.ntp.br",
		"b.st1.ntp.br",
		"pool.ntp.org",
	}
	for _, srv := range servers {
		t, err = queryNTP(srv)
		if err == nil {
			return t, srv, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("todos os servidores NTP falharam; último erro: %w", err)
}

// queryNTP implementa um cliente NTPv4 mínimo usando UDP na porta 123.
// O timestamp retornado é o "transmit timestamp" do servidor (bytes 40-47 do pacote).
func queryNTP(server string) (time.Time, error) {
	conn, err := net.DialTimeout("udp", server+":123", 5*time.Second)
	if err != nil {
		return time.Time{}, fmt.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Pacote NTP de 48 bytes: LI=0, VN=4 (versão 4), Mode=3 (cliente) → primeiro byte = 0x23.
	req := make([]byte, 48)
	req[0] = 0x23

	if _, err = conn.Write(req); err != nil {
		return time.Time{}, fmt.Errorf("write NTP: %w", err)
	}

	resp := make([]byte, 48)
	if _, err = conn.Read(resp); err != nil {
		return time.Time{}, fmt.Errorf("read NTP: %w", err)
	}

	// O transmit timestamp ocupa bytes 40-43 (segundos desde 1900-01-01 UTC).
	secs := binary.BigEndian.Uint32(resp[40:44])
	// Diferença em segundos entre época NTP (1900) e época Unix (1970): 70 anos.
	const ntpUnixDelta = 2208988800
	return time.Unix(int64(secs)-ntpUnixDelta, 0).UTC(), nil
}
