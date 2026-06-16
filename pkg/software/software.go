//go:build windows

package software

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows/registry"
)

type installedApp struct {
	DisplayName     string
	DisplayVersion  string
	Publisher       string
	InstallDate     string
	InstallLocation string
	Source          string
}

// uninstallHives define os três locais do Registro onde o Windows lista programas instalados.
var uninstallHives = []struct {
	root   registry.Key
	path   string
	source string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM x64"},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM x86"},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKCU"},
}

// Collect lê as três chaves Uninstall do Registro em paralelo e salva em installed_software.csv.
func Collect(auditDir string) error {
	type result struct {
		apps []installedApp
		err  error
	}
	ch := make(chan result, len(uninstallHives))

	for _, hive := range uninstallHives {
		hive := hive
		go func() {
			apps, err := readHive(hive.root, hive.path, hive.source)
			ch <- result{apps, err}
		}()
	}

	var allApps []installedApp
	var errs []string
	for range uninstallHives {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err.Error())
			continue
		}
		allApps = append(allApps, r.apps...)
	}

	if err := writeCSV(auditDir, allApps); err != nil {
		return err
	}

	if len(errs) > 0 {
		// Erros parciais (ex: hive HKCU indisponível) não abortam — apenas avisam.
		return fmt.Errorf("leitura parcial do registro: %s", joinErrors(errs))
	}
	return nil
}

// readHive itera sobre as subchaves de um hive Uninstall e extrai metadados de cada programa.
func readHive(root registry.Key, path, source string) ([]installedApp, error) {
	k, err := registry.OpenKey(root, path, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("abrir chave %s\\%s: %w", source, path, err)
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("listar subchaves de %s: %w", source, err)
	}

	var mu sync.Mutex
	var apps []installedApp
	var wg sync.WaitGroup

	// Leitura de subchaves em paralelo via goroutines (cada subchave é um programa).
	sem := make(chan struct{}, 16) // limita concorrência para não estrangular o registro
	for _, name := range subkeys {
		name := name
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			sk, err := registry.OpenKey(k, name, registry.READ)
			if err != nil {
				return
			}
			defer sk.Close()

			displayName, _, _ := sk.GetStringValue("DisplayName")
			if displayName == "" {
				return // entradas sem nome visível são componentes internos; ignorar
			}

			app := installedApp{
				Source:      source,
				DisplayName: displayName,
			}
			app.DisplayVersion, _, _ = sk.GetStringValue("DisplayVersion")
			app.Publisher, _, _ = sk.GetStringValue("Publisher")
			app.InstallDate, _, _ = sk.GetStringValue("InstallDate")
			app.InstallLocation, _, _ = sk.GetStringValue("InstallLocation")

			mu.Lock()
			apps = append(apps, app)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return apps, nil
}

func writeCSV(auditDir string, apps []installedApp) error {
	outPath := filepath.Join(auditDir, "installed_software.csv")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("criar installed_software.csv: %w", err)
	}
	defer f.Close()

	// BOM UTF-8: sinaliza para Excel/Notepad do Windows que o arquivo é UTF-8,
	// evitando que caracteres como ™ ® © sejam lidos como Windows-1252.
	f.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{"DisplayName", "DisplayVersion", "Publisher", "InstallDate", "InstallLocation", "Source"})
	for _, a := range apps {
		_ = w.Write([]string{a.DisplayName, a.DisplayVersion, a.Publisher, a.InstallDate, a.InstallLocation, a.Source})
	}
	return w.Error()
}

func joinErrors(errs []string) string {
	s := ""
	for i, e := range errs {
		if i > 0 {
			s += "; "
		}
		s += e
	}
	return s
}
