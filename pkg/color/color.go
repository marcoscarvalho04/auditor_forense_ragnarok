//go:build windows

package color

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const enableVirtualTerminalProcessing uint32 = 0x0004

// EnableVT habilita o processamento de sequências ANSI no console do Windows 10+.
func EnableVT() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	_ = windows.SetConsoleMode(handle, mode|enableVirtualTerminalProcessing)
}

// Códigos ANSI de formatação.
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	White  = "\033[97m"
)

func Success(msg string) { fmt.Printf("%s%s[✓] %s%s\n", Bold, Green, msg, Reset) }
func Fail(msg string)    { fmt.Printf("%s%s[✗] %s%s\n", Bold, Red, msg, Reset) }
func Info(msg string)    { fmt.Printf("%s[→] %s%s\n", Cyan, msg, Reset) }
func Warn(msg string)    { fmt.Printf("%s[!] %s%s\n", Yellow, msg, Reset) }
func Step(msg string)    { fmt.Printf("\n%s%s%s%s\n", Bold, White, msg, Reset) }
