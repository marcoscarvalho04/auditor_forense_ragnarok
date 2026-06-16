//go:build windows

package artifacts

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const prefetchDir = `C:\Windows\Prefetch`

// Collect executa a coleta de Prefetch e Tarefas Agendadas em paralelo.
func Collect(auditDir string) error {
	type result struct {
		label string
		err   error
	}
	ch := make(chan result, 2)

	go func() {
		err := collectPrefetch(auditDir)
		ch <- result{"Prefetch", err}
	}()

	go func() {
		err := collectScheduledTasks(auditDir)
		ch <- result{"Tarefas Agendadas", err}
	}()

	var errs []string
	for range 2 {
		r := <-ch
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.label, r.err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// collectPrefetch lê C:\Windows\Prefetch e registra todos os arquivos .pf em CSV.
func collectPrefetch(auditDir string) error {
	entries, err := os.ReadDir(prefetchDir)
	if err != nil {
		return fmt.Errorf("ler %s: %w", prefetchDir, err)
	}

	outPath := filepath.Join(auditDir, "prefetch_records.csv")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("criar prefetch_records.csv: %w", err)
	}
	defer f.Close()

	f.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8 para compatibilidade com Excel/Notepad

	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{"FileName", "SizeBytes", "CreatedAt", "ModifiedAt"})

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pf") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(prefetchDir, e.Name())
		_ = w.Write([]string{
			e.Name(),
			fmt.Sprintf("%d", info.Size()),
			fileCreationTime(fullPath),
			info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return w.Error()
}

// collectScheduledTasks executa schtasks e salva a saída em scheduled_tasks.csv.
func collectScheduledTasks(auditDir string) error {
	cmd := exec.Command("schtasks", "/query", "/fo", "CSV", "/v")
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return fmt.Errorf("schtasks falhou: %w", err)
	}
	outPath := filepath.Join(auditDir, "scheduled_tasks.csv")
	return os.WriteFile(outPath, out, 0644)
}

// fileCreationTime usa a API Win32 GetFileInformationByHandle para obter a data de criação,
// que não está exposta pelo FileInfo padrão do Go no Windows.
func fileCreationTime(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		return ""
	}

	// FILETIME é um uint64 de intervalos de 100ns desde 1601-01-01 UTC.
	ft := (*syscall.Filetime)(unsafe.Pointer(&fi.CreationTime))
	return time.Unix(0, ft.Nanoseconds()).Local().Format("2006-01-02 15:04:05")
}
