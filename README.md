# Auditor Forense — Anti-Cheat False Positive Evidence Collector

Utilitário de linha de comando para Windows que automatiza a coleta de evidências digitais em casos de **falso positivo de sistemas Anti-Cheat**, gerando um pacote forense rastreável e adequado para embasar pareceres técnicos judiciais ou extrajudiciais.

## Motivação

Sistemas Anti-Cheat (EasyAntiCheat, BattlEye, VAC, etc.) operam com acesso elevado ao sistema operacional e, em determinadas circunstâncias, emitem banimentos ou bloqueios indevidos — os chamados **falsos positivos**. Nesses casos, o jogador prejudicado frequentemente não dispõe de evidências técnicas organizadas para contestar a decisão junto à desenvolvedora, distribuidora ou instâncias judiciais.

Este utilitário resolve esse problema ao coletar, de forma estruturada e reproduzível, os artefatos do sistema relevantes para demonstrar que nenhum software proibido estava presente ou em execução no momento do incidente.

## O que é coletado

| Artefato | Arquivo gerado |
|---|---|
| Hostname, usuário, versão do Windows, timestamp NTP | `system_info.txt` |
| Processos em execução (via WMI Win32_Process) | `process_dump.csv` |
| Programas instalados (3 chaves do Registro) | `installed_software.csv` |
| Histórico de execução (Prefetch) | `prefetch_records.csv` |
| Tarefas agendadas | `scheduled_tasks.csv` |
| Log de eventos System | `system_log.evtx` |
| Log de eventos Application | `application_log.evtx` |
| Conexões de rede ativas | `netstat_connections.txt` |
| Inventário completo do jogo com SHA-256 por arquivo | `game_files_inventory.txt` |
| Artefatos do Anti-Cheat (dumps, logs, crashlogs) | `Game_Logs/` |

Ao final, todos os arquivos são compactados em um `.zip` com **hash SHA-256** registrado, formando uma cadeia de custódia verificável.

## Requisitos

- Windows 10 ou superior (x64)
- Go 1.22+ (apenas para compilação)
- Execução como **Administrador** (obrigatório — leitura de Prefetch e WMI exige elevação)

## Compilação

```powershell
git clone <repositório>
cd auditor-processos
go mod tidy
go build -ldflags="-s -w" -o auditor_forense.exe .
```

## Uso

1. Clique com o botão direito em `auditor_forense.exe`
2. Selecione **Executar como Administrador**
3. Siga as instruções no terminal
4. Ao final, o arquivo `Evidencias_Ragnarok_<timestamp>.zip` e seu `.sha256.txt` estarão em:

```
<pasta do executável>/
└── evidencias_ragnarok/
    ├── Evidencias_Ragnarok_<timestamp>.zip
    └── Evidencias_Ragnarok_<timestamp>.zip.sha256.txt
```

## Cadeia de custódia

O hash SHA-256 do arquivo ZIP é exibido em destaque no terminal ao término da execução. Recomenda-se:

- Gravar a tela durante a execução do utilitário
- Registrar o hash em documento com data e assinatura
- Não modificar o arquivo ZIP após a geração (o hash deixaria de ser válido)

## Estrutura do projeto

```
auditor-processos/
├── main.go                  Orquestrador principal
└── pkg/
    ├── admin/               Verificação de privilégios elevados
    ├── color/               Saída colorida no terminal (ANSI/VT100)
    ├── sysinfo/             Metadados do sistema + cliente NTP
    ├── processes/           Dump de processos via WMI
    ├── software/            Inventário de software via Registro
    ├── artifacts/           Prefetch e tarefas agendadas
    ├── winlogs/             Logs de eventos e netstat
    ├── game/                Coleta de artefatos do jogo e Anti-Cheat
    └── packaging/           Empacotamento ZIP e cadeia de custódia
```

## Aviso legal

Este utilitário coleta informações do sistema local para fins exclusivamente defensivos e de documentação técnica. Nenhum dado é transmitido para servidores externos. O timestamp NTP é obtido de servidores públicos brasileiros (`st1.ntp.br`) apenas para registrar a hora oficial no momento da coleta.
