//go:build windows

package packaging

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Pack compacta auditDir em um ZIP em baseDir (pasta do executável),
// calcula o SHA-256 do arquivo resultante e remove a pasta temporária.
// Retorna o caminho absoluto do ZIP e o hash SHA-256 em hex.
func Pack(auditDir, baseDir, timestamp string) (zipPath, hashHex string, err error) {
	zipName := fmt.Sprintf("Evidencias_Ragnarok_%s.zip", timestamp)
	zipPath = filepath.Join(baseDir, zipName)

	if err := createZIP(auditDir, zipPath); err != nil {
		return "", "", fmt.Errorf("criar ZIP: %w", err)
	}

	hashHex, err = sha256File(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("calcular SHA-256 do ZIP: %w", err)
	}

	// Salva o hash dentro do ZIP seria impossível sem reabrir; grava em arquivo paralelo.
	hashFilePath := zipPath + ".sha256.txt"
	hashContent := fmt.Sprintf("SHA-256: %s  %s\n", hashHex, zipName)
	if wErr := os.WriteFile(hashFilePath, []byte(hashContent), 0644); wErr != nil {
		// Não fatal; o hash será exibido no terminal de qualquer forma.
		fmt.Printf("[!] Não foi possível gravar arquivo .sha256.txt: %v\n", wErr)
	}

	// Remove a pasta de auditoria bruta; mantém apenas o ZIP e o .sha256.txt.
	if err := os.RemoveAll(auditDir); err != nil {
		fmt.Printf("[!] Limpeza da pasta temporária falhou: %v\n", err)
	}

	return zipPath, hashHex, nil
}

// createZIP percorre auditDir recursivamente e adiciona cada arquivo ao arquivo ZIP,
// preservando a estrutura de diretórios relativa.
func createZIP(sourceDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("criar arquivo ZIP: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	err = filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Caminho relativo ao sourceDir — arquivos ficam na raiz do ZIP.
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		zipEntry := strings.ReplaceAll(rel, string(os.PathSeparator), "/")

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipEntry
		header.Method = zip.Deflate

		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		_, err = io.Copy(w, src)
		return err
	})

	return err
}

// sha256File lê o arquivo inteiro e retorna o hash SHA-256 em hexadecimal.
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
