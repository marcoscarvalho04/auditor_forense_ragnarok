//go:build windows

package processes

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/StackExchange/wmi"
)

// win32Process mapeia os campos relevantes da classe WMI Win32_Process.
// Ponteiros são usados em campos que podem ser NULL (processos de sistema).
type win32Process struct {
	ProcessId      uint32
	Name           string
	ExecutablePath *string
	CommandLine    *string
	CreationDate   *string // Formato DMTF: YYYYMMDDHHmmss.ffffff±UUU
}

// Collect consulta o WMI para listar todos os processos em execução e salva em process_dump.csv.
// Requer que o processo esteja rodando como Administrador para obter todos os campos.
func Collect(auditDir string) error {
	var procs []win32Process

	// Query literal necessária: CreateQuery deriva o nome da classe do tipo Go,
	// gerando "FROM win32Process" (inválido) em vez de "FROM Win32_Process".
	const q = `SELECT ProcessId, Name, ExecutablePath, CommandLine, CreationDate FROM Win32_Process`
	if err := wmi.Query(q, &procs); err != nil {
		return fmt.Errorf("consulta WMI Win32_Process: %w", err)
	}

	outPath := filepath.Join(auditDir, "process_dump.csv")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("criar process_dump.csv: %w", err)
	}
	defer f.Close()

	f.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8 para compatibilidade com Excel/Notepad

	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{"ProcessId", "Name", "ExecutablePath", "CommandLine", "CreationDate"})

	for _, p := range procs {
		row := []string{
			strconv.FormatUint(uint64(p.ProcessId), 10),
			p.Name,
			derefStr(p.ExecutablePath),
			derefStr(p.CommandLine),
			parseDMTFDate(derefStr(p.CreationDate)),
		}
		_ = w.Write(row)
	}

	if err := w.Error(); err != nil {
		return fmt.Errorf("escrever CSV de processos: %w", err)
	}
	return nil
}

// derefStr retorna a string apontada ou vazio se o ponteiro for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseDMTFDate converte o formato DMTF do WMI (YYYYMMDDHHmmss.ffffff±UUU)
// para uma string legível. Retorna a string original em caso de erro de parsing.
func parseDMTFDate(s string) string {
	if len(s) < 14 {
		return s
	}
	t, err := time.ParseInLocation("20060102150405", s[:14], time.Local)
	if err != nil {
		return s
	}
	return t.Format("2006-01-02 15:04:05")
}
