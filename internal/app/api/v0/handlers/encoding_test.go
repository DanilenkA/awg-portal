package handlers

import "testing"

// TestBase64UrlDecode покрывает регрессионные случаи бага 3:
//   - frontend-кодирование с padding (YXdnMA--, RGVmYXVsdA--);
//   - frontend-кодирование с одним символом padding (YXdnMTA- для "awg10" —
//     раньше URLEncoding возвращал "awg10>", а isPrintableASCII пропускал
//     это как валидный результат);
//   - URL-safe без padding (d2cw);
//   - URL-safe с заменой символов (test-1 → dGVzdC0x);
//   - plain-text ID (wg0, test-1) — должны возвращаться как есть;
//   - пустая строка;
//   - строки с неподходящими символами.
func TestBase64UrlDecode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Frontend-кодирование (btoa + замены): padding через '-'
		{"awg0 frontend", "YXdnMA--", "awg0"},
		{"awg1 frontend", "YXdnMQ--", "awg1"},
		{"awg10 frontend (1 padding)", "YXdnMTA-", "awg10"},
		{"Default frontend", "RGVmYXVsdA--", "Default"},

		// Frontend-кодирование с заменой / и +
		{"John Doe frontend", "Sm9obiBEb2U-", "John Doe"},
		{"John.Doe frontend", "Sm9obi5Eb2U-", "John.Doe"},
		{"vpn-server-01 frontend", "dnBuLXNlcnZlci0wMQ--", "vpn-server-01"},

		// URL-safe без padding
		{"wg0 raw url-safe", "d2cw", "wg0"},
		{"test-1 raw url-safe", "dGVzdC0x", "test-1"},

		// Plain-text ID — должны возвращаться как есть
		{"plain wg0", "wg0", "wg0"},
		{"plain awg0", "awg0", "awg0"},
		{"plain test-1", "test-1", "test-1"},
		{"plain user.with.dot", "user.with.dot", "user.with.dot"},
		{"plain _user_id", "_user_id", "_user_id"},

		// Пустая строка
		{"empty", "", ""},

		// Содержит символы вне base64-алфавита — возвращаем как есть
		{"has plus", "+bad+", "+bad+"},
		{"has space", "wg 0", "wg 0"},
		{"has at", "admin@example.com", "admin@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Base64UrlDecode(tc.in)
			if got != tc.want {
				t.Errorf("Base64UrlDecode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBase64UrlDecodeNoGarbageFromLenientURLEncoding — регрессионный тест бага 3:
// гарантирует, что мы НЕ возвращаем мусорные результаты от Go URLEncoding для
// строк с '-'-padding (например, "YXdnMA--"). Раньше URLEncoding.DecodeString
// возвращал "awg0\x0f\xbe", а вторая попытка фикса для "YXdnMTA-" отдавала
// "awg10>" (печатный ASCII, но с лишним '>'). Этот тест фиксирует, что
// правильный фронтенд-путь отрабатывает первым и не даёт мусору "просочиться".
func TestBase64UrlDecodeNoGarbageFromLenientURLEncoding(t *testing.T) {
	// Проверяем, что для frontend-кодирования результат в точности равен
	// ожидаемому plain-text, без хвостовых мусорных байтов.
	cases := []struct {
		in   string
		want string
	}{
		{"YXdnMA--", "awg0"},
		{"YXdnMTA-", "awg10"}, // не "awg10>"
		{"RGVmYXVsdA--", "Default"},
	}
	for _, c := range cases {
		got := Base64UrlDecode(c.in)
		if got != c.want {
			t.Errorf("Base64UrlDecode(%q) = %q, want %q (возможен мусор от URLEncoding)",
				c.in, got, c.want)
		}
		// Дополнительно: длина результата должна быть <= длины plain-text-аналога
		// из 4-байтового base64 (на каждыe 4 base64-символа — 3 байта plain).
		if len(got) > len(c.want) {
			t.Errorf("Base64UrlDecode(%q) = %q (len=%d) — результат длиннее ожидаемого %q (len=%d)",
				c.in, got, len(got), c.want, len(c.want))
		}
	}
}