package main

import "fmt"

const (
	ansiReset    = "\x1b[0m"
	ansiDim      = "\x1b[2m"
	ansiRed      = "\x1b[31m"
	ansiGreen    = "\x1b[32m"
	ansiYellow   = "\x1b[33m"
	ansiCyan     = "\x1b[36m"
	ansiBoldCyan = "\x1b[1;36m"
)

func ansiText(enabled bool, style, text string) string {
	if !enabled || text == "" {
		return text
	}
	return style + text + ansiReset
}

func printConsoleTitle(title, divider string) {
	colors := stdoutIsTTY()
	fmt.Println(ansiText(colors, ansiBoldCyan, title))
	fmt.Println(ansiText(colors, ansiCyan, divider))
}

func printConsoleSection(title string) {
	fmt.Printf("%s:\n", ansiText(stdoutIsTTY(), ansiBoldCyan, title))
}

func consoleMetric(value any) string {
	return ansiText(stdoutIsTTY(), ansiGreen, fmt.Sprint(value))
}
