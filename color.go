package main

import "fmt"

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
)

func blueString(s string) string {
	return fmt.Sprintf("%s%s%s", colorBlue, s, colorReset)
}

func yellowString(s string) string {
	return fmt.Sprintf("%s%s%s", colorYellow, s, colorReset)
}

func greenString(s string) string {
	return fmt.Sprintf("%s%s%s", colorGreen, s, colorReset)
}
