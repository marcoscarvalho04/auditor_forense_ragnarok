//go:build windows

package winlogs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Collect exporta os logs System e Application via wevtutil (em paralelo)
// e captura as conexões de rede via netstat. Tudo salvo no auditDir.
func Collect(auditDir string) error {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	type result struct {
		label string
		err   error
	}
	ch := make(chan result, 3)
	var wg sync.WaitGroup

	// Exportação do log System
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := exportEventLog("System", tempDir, auditDir)
		ch <- result{"Log System", err}
	}()

	// Exportação do log Application
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := exportEventLog("Application", tempDir, auditDir)
		ch <- result{"Log Application", err}
	}()

	// Captura do netstat
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := collectNetstat(auditDir)
		ch <- result{"Netstat", err}
	}()

	// Aguarda todas as goroutines terminarem antes de fechar o canal.
	go func() {
		wg.Wait()
		close(ch)
	}()

	var errs []string
	for r := range ch {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.label, r.err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// exportEventLog executa wevtutil epl para exportar um canal de log para .evtx
// no TEMP e depois move o arquivo para o diretório de auditoria.
func exportEventLog(channel, tempDir, auditDir string) error {
	filename := strings.ToLower(channel) + "_log.evtx"
	tempPath := filepath.Join(tempDir, filename)
	destPath := filepath.Join(auditDir, filename)

	// Remove arquivo temporário anterior caso exista (wevtutil falha se já existir).
	_ = os.Remove(tempPath)

	cmd := exec.Command("wevtutil", "epl", channel, tempPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wevtutil epl %s: %w — saída: %s", channel, err, strings.TrimSpace(string(out)))
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		// Rename pode falhar entre drives diferentes; tenta cópia manual.
		return copyFile(tempPath, destPath)
	}
	return nil
}

// collectNetstat executa netstat -ano e salva a saída em netstat_connections.txt.
func collectNetstat(auditDir string) error {
	cmd := exec.Command("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("netstat: %w", err)
	}
	outPath := filepath.Join(auditDir, "netstat_connections.txt")
	return os.WriteFile(outPath, out, 0644)
}

// copyFile copia src para dst byte a byte; usado quando os.Rename falha entre volumes.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("ler %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("escrever %s: %w", dst, err)
	}
	_ = os.Remove(src)
	return nil
}
