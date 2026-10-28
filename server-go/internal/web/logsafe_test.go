package web

import (
	"strings"
	"testing"
)

func TestLogSafeValueStripsControlCharacters(t *testing.T) {
	got := logSafeValue("/assets/%0A%5Bforged%5D%20x")
	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("valor de log conserva caracteres de control: %q", got)
	}
}

func TestLogSafeValueQuotesForInjection(t *testing.T) {
	got := logSafeValue("a\n[forged] x")
	if strings.Contains(got, "\n") {
		t.Fatalf("el salto de linea sobrevivio: %q", got)
	}
	if !strings.Contains(got, `\n`) {
		t.Fatalf("el salto de linea no se escapo de forma visible: %q", got)
	}
}

func TestLogSafeValueTruncatesLongInput(t *testing.T) {
	got := logSafeValue(strings.Repeat("a", 2048))
	if len(got) > maxLogValueLen+8 {
		t.Fatalf("el valor de log no se truncó: len = %d", len(got))
	}
}
