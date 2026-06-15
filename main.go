//go:build windows

// auditor-processos — Utilitário de extração forense para Windows.
//
// Uso: Execute como Administrador.
//   GOOS=windows GOARCH=amd64 go build -o auditor_forense.exe .
//
// Saída: <pasta do exe>\Evidencias_Ragnarok_<timestamp>.zip
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"auditor-processos/pkg/admin"
	"auditor-processos/pkg/artifacts"
	"auditor-processos/pkg/color"
	"auditor-processos/pkg/game"
	"auditor-processos/pkg/packaging"
	"auditor-processos/pkg/processes"
	"auditor-processos/pkg/software"
	"auditor-processos/pkg/sysinfo"
	"auditor-processos/pkg/winlogs"
)

func main() {
	color.EnableVT()
	printBanner()

	// Verificação de privilégios — aborta antes de qualquer coleta se não for Admin.
	if !admin.IsElevated() {
		color.Fail("Este utilitário requer privilégios de Administrador.")
		fmt.Fprintln(os.Stderr, "       Clique com o botão direito no executável → 'Executar como Administrador'.")
		os.Exit(1)
	}
	color.Success("Privilégios de Administrador confirmados.")

	// Resolve o diretório onde o executável está instalado.
	// Usado como raiz para a pasta de trabalho e para o ZIP final.
	exeDir, err := executableDir()
	if err != nil {
		fatalf("Não foi possível determinar o diretório do executável: %v", err)
	}

	// Pasta permanente de saída — onde o ZIP e o .sha256.txt ficam após a coleta.
	evidenciasDir := filepath.Join(exeDir, "evidencias_ragnarok")
	if err := os.MkdirAll(evidenciasDir, 0755); err != nil {
		fatalf("Falha ao criar pasta de saída '%s': %v", evidenciasDir, err)
	}

	// Pasta temporária de trabalho — removida após o empacotamento.
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	auditDir := filepath.Join(evidenciasDir, "tmp_"+timestamp)
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		fatalf("Falha ao criar diretório de trabalho '%s': %v", auditDir, err)
	}
	color.Info(fmt.Sprintf("Saída       : %s", evidenciasDir))
	color.Info(fmt.Sprintf("Trabalho tmp: %s", auditDir))

	// ── Passo 1 ── Metadados do Sistema ──────────────────────────────────────
	color.Step("Passo 1/7 — Metadados do Sistema")
	if err := sysinfo.Collect(auditDir); err != nil {
		color.Warn("Erro parcial em metadados do sistema: " + err.Error())
	} else {
		color.Success("system_info.txt gerado.")
	}

	// ── Passo 2 ── Dump de Processos (WMI) ───────────────────────────────────
	color.Step("Passo 2/7 — Dump de Processos via WMI")
	if err := processes.Collect(auditDir); err != nil {
		color.Warn("Erro ao coletar processos WMI: " + err.Error())
	} else {
		color.Success("process_dump.csv gerado.")
	}

	// ── Passo 3 ── Inventário de Software (Registro) ──────────────────────────
	color.Step("Passo 3/7 — Inventário de Software via Registro")
	if err := software.Collect(auditDir); err != nil {
		color.Warn("Erro (parcial) no inventário de software: " + err.Error())
	} else {
		color.Success("installed_software.csv gerado.")
	}

	// ── Passo 4 ── Artefatos de Execução (Prefetch + Tarefas) ────────────────
	color.Step("Passo 4/7 — Artefatos de Execução (Prefetch + Tarefas Agendadas)")
	if err := artifacts.Collect(auditDir); err != nil {
		color.Warn("Erro nos artefatos de execução: " + err.Error())
	} else {
		color.Success("prefetch_records.csv e scheduled_tasks.csv gerados.")
	}

	// ── Passo 5 ── Logs do Windows + Rede ────────────────────────────────────
	color.Step("Passo 5/7 — Logs do Windows (wevtutil) e Conexões de Rede (netstat)")
	if err := winlogs.Collect(auditDir); err != nil {
		color.Warn("Erro (parcial) nos logs/rede: " + err.Error())
	} else {
		color.Success("system_log.evtx, application_log.evtx e netstat_connections.txt gerados.")
	}

	// ── Passo 6 ── Escopo do Jogo (Ragnarök Online) ───────────────────────────
	color.Step("Passo 6/7 — Artefatos do Ragnarök Online / Anti-Cheat")
	roPath := promptRagnarokPath()
	if err := game.Collect(auditDir, roPath); err != nil {
		color.Warn("Erro nos artefatos do jogo: " + err.Error())
	} else {
		color.Success("Pasta Game_Logs/ e game_executable_hash.txt gerados.")
	}

	// ── Passo 7 ── Empacotamento e Cadeia de Custódia ─────────────────────────
	color.Step("Passo 7/7 — Empacotando evidências e calculando SHA-256 (cadeia de custódia)")
	zipPath, sha256sum, err := packaging.Pack(auditDir, evidenciasDir, timestamp)
	if err != nil {
		fatalf("Falha crítica ao empacotar evidências: %v", err)
	}

	printFinalReport(zipPath, sha256sum)
}

func promptRagnarokPath() string {
	fmt.Printf("\n%s%sCaminho da pasta de instalação do Ragnarök Online%s\n", color.Bold, color.Yellow, color.Reset)
	fmt.Printf("%s(ex: C:\\Program Files (x86)\\Ragnarok): %s", color.Yellow, color.Reset)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func printBanner() {
	sep := strings.Repeat("═", 68)
	fmt.Printf("\n%s%s╔%s╗%s\n", color.Bold, color.Cyan, sep, color.Reset)
	fmt.Printf("%s%s║  AUDITOR FORENSE — RAGNARÖK ONLINE ANTI-CHEAT                    ║%s\n", color.Bold, color.Cyan, color.Reset)
	fmt.Printf("%s%s║  Coleta de Evidências para Parecer Técnico Judicial               ║%s\n", color.Bold, color.Cyan, color.Reset)
	fmt.Printf("%s%s╚%s╝%s\n\n", color.Bold, color.Cyan, sep, color.Reset)
}

func printFinalReport(zipPath, sha256sum string) {
	sep := strings.Repeat("─", 68)
	fmt.Printf("\n%s%s%s%s\n", color.Bold, color.Green, sep, color.Reset)
	fmt.Printf("%s%s  ✓  COLETA DE EVIDÊNCIAS CONCLUÍDA COM SUCESSO%s\n", color.Bold, color.Green, color.Reset)
	fmt.Printf("%s%s%s%s\n", color.Bold, color.Green, sep, color.Reset)

	fmt.Printf("\n  %sArquivo ZIP:%s\n  %s\n", color.Cyan, color.Reset, zipPath)
	fmt.Printf("\n  %sSHA-256 (Cadeia de Custódia):%s\n  %s%s%s\n",
		color.Cyan, color.Reset, color.Bold, sha256sum, color.Reset)

	fmt.Printf("\n%s%s%s%s\n", color.Bold, color.Green, sep, color.Reset)
	fmt.Printf("\n%s%s  ► REGISTRE ESTE HASH EM VÍDEO OU DOCUMENTO FORMAL  ◄%s\n\n",
		color.Bold, color.Yellow, color.Reset)
}

// executableDir retorna o diretório absoluto onde o binário está instalado.
// EvalSymlinks resolve links simbólicos (e o diretório temporário do `go run`).
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func fatalf(format string, args ...any) {
	color.Fail(fmt.Sprintf(format, args...))
	os.Exit(1)
}
