//go:build windows

package game

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const hashWorkers = 8 // goroutines paralelas para hashing (I/O bound)

// antiCheatDirs são subpastas de Anti-Cheat conhecidas.
var antiCheatDirs = []string{"EasyAntiCheat", "EAC", "anticheat", "anti-cheat"}

// artifactExtensions são extensões de arquivos de interesse forense.
var artifactExtensions = []string{".dmp", ".log", ".txt"}

// fileEntry representa um arquivo encontrado na pasta do jogo.
type fileEntry struct {
	absPath string
	relPath string
	size    int64
}

// hashResult é o resultado do hashing de um fileEntry.
type hashResult struct {
	entry  fileEntry
	sha256 string
	err    error
}

// Collect executa o inventário completo da pasta do jogo e copia artefatos de Anti-Cheat.
func Collect(auditDir, roPath string) error {
	if roPath == "" {
		return fmt.Errorf("caminho não fornecido")
	}
	info, err := os.Stat(roPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("caminho '%s' não existe ou não é um diretório: %v", roPath, err)
	}

	gameLogsDir := filepath.Join(auditDir, "Game_Logs")
	if err := os.MkdirAll(gameLogsDir, 0755); err != nil {
		return fmt.Errorf("criar Game_Logs: %w", err)
	}

	// Coleta lista de todos os arquivos antes de iniciar o hashing.
	fmt.Printf("       Varrendo arquivos em %s...\n", roPath)
	entries, err := walkFiles(roPath)
	if err != nil {
		return fmt.Errorf("varredura de arquivos: %w", err)
	}
	fmt.Printf("       %d arquivo(s) encontrado(s). Calculando hashes (workers: %d)...\n", len(entries), hashWorkers)

	// Hashing paralelo via pool de workers.
	results := hashAll(entries)

	// Escreve o inventário completo em game_files_inventory.txt.
	if err := writeInventory(auditDir, roPath, results); err != nil {
		return fmt.Errorf("escrever inventário: %w", err)
	}

	// Copia artefatos de Anti-Cheat para Game_Logs/.
	copied, err := copyArtifacts(roPath, gameLogsDir)
	if err != nil {
		return fmt.Errorf("copiar artefatos: %w", err)
	}
	if copied == 0 {
		note := "Nenhum arquivo de dump/log de Anti-Cheat encontrado na pasta de instalação.\n"
		_ = os.WriteFile(filepath.Join(gameLogsDir, "NENHUM_ARTEFATO_ENCONTRADO.txt"), []byte(note), 0644)
	}

	return nil
}

// walkFiles percorre roPath recursivamente e retorna todos os arquivos regulares.
func walkFiles(roPath string) ([]fileEntry, error) {
	var entries []fileEntry
	err := filepath.WalkDir(roPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(roPath, path)
		entries = append(entries, fileEntry{
			absPath: path,
			relPath: rel,
			size:    info.Size(),
		})
		return nil
	})
	return entries, err
}

// hashAll distribui o hashing entre hashWorkers goroutines e coleta os resultados.
func hashAll(entries []fileEntry) []hashResult {
	jobs := make(chan fileEntry, len(entries))
	resultCh := make(chan hashResult, len(entries))

	var wg sync.WaitGroup
	for range hashWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				sum, err := sha256File(e.absPath)
				resultCh <- hashResult{entry: e, sha256: sum, err: err}
			}
		}()
	}

	for _, e := range entries {
		jobs <- e
	}
	close(jobs)

	// Aguarda workers terminarem e fecha o canal de resultados.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var results []hashResult
	for r := range resultCh {
		results = append(results, r)
	}

	// Ordena por caminho relativo para leitura determinística no laudo.
	sort.Slice(results, func(i, j int) bool {
		return results[i].entry.relPath < results[j].entry.relPath
	})
	return results
}

// writeInventory grava game_files_inventory.txt com cabeçalho e tabela de hashes.
func writeInventory(auditDir, roPath string, results []hashResult) error {
	outPath := filepath.Join(auditDir, "game_files_inventory.txt")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// BOM UTF-8 para compatibilidade com editores Windows.
	f.Write([]byte{0xEF, 0xBB, 0xBF})

	var totalSize int64
	var errCount int
	for _, r := range results {
		totalSize += r.entry.size
		if r.err != nil {
			errCount++
		}
	}

	sep := strings.Repeat("=", 120)
	fmt.Fprintf(f, "%s\n", sep)
	fmt.Fprintf(f, "  INVENTÁRIO FORENSE COMPLETO — RAGNARÖK ONLINE\n")
	fmt.Fprintf(f, "%s\n", sep)
	fmt.Fprintf(f, "  Pasta raiz : %s\n", roPath)
	fmt.Fprintf(f, "  Gerado em  : %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "  Total      : %d arquivo(s) | %s\n", len(results), formatSize(totalSize))
	if errCount > 0 {
		fmt.Fprintf(f, "  Erros      : %d arquivo(s) não puderam ser lidos\n", errCount)
	}
	fmt.Fprintf(f, "%s\n\n", sep)

	// Cabeçalho da tabela.
	fmt.Fprintf(f, "%-64s  %-12s  %s\n", "SHA-256", "Tamanho", "Caminho Relativo")
	fmt.Fprintf(f, "%s  %s  %s\n",
		strings.Repeat("-", 64),
		strings.Repeat("-", 12),
		strings.Repeat("-", 40),
	)

	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(f, "%-64s  %-12s  %s\n",
				"(ERRO: "+r.err.Error()+")",
				formatSize(r.entry.size),
				r.entry.relPath,
			)
			continue
		}
		fmt.Fprintf(f, "%s  %-12s  %s\n",
			r.sha256,
			formatSize(r.entry.size),
			r.entry.relPath,
		)
	}

	fmt.Fprintf(f, "\n%s\n", sep)
	fmt.Fprintf(f, "  FIM DO INVENTÁRIO — %d arquivo(s) — %s\n", len(results), formatSize(totalSize))
	fmt.Fprintf(f, "%s\n", sep)

	return nil
}

// copyArtifacts copia arquivos de interesse de subpastas Anti-Cheat para destDir.
func copyArtifacts(roPath, destDir string) (int, error) {
	copied := 0
	err := filepath.WalkDir(roPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(roPath, path)
		parts := strings.Split(rel, string(os.PathSeparator))

		inAntiCheatDir := false
		for _, part := range parts[:len(parts)-1] {
			for _, acd := range antiCheatDirs {
				if strings.EqualFold(part, acd) {
					inAntiCheatDir = true
				}
			}
		}

		name := strings.ToLower(d.Name())
		isCrashLog := name == "crashlog.txt" || name == "crash.log"
		hasArtifactExt := false
		for _, ext := range artifactExtensions {
			if strings.HasSuffix(name, ext) {
				hasArtifactExt = true
			}
		}

		if (!inAntiCheatDir && !isCrashLog) || (!hasArtifactExt && !isCrashLog) {
			return nil
		}

		destPath := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			if os.WriteFile(destPath, data, 0644) == nil {
				copied++
			}
		}
		return nil
	})
	return copied, err
}

// sha256File calcula o SHA-256 de um arquivo via streaming (suporta arquivos grandes como .grf).
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// formatSize converte bytes para uma representação legível (B, KB, MB, GB).
func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGT"[exp])
}
