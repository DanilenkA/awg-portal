package handlers

import (
	"encoding/base64"
	"regexp"
	"strings"
)

// base64LikeRegex описывает набор символов, которые могут встречаться в base64-строке:
// алфавит A-Z, a-z, 0-9 плюс URL-safe замены ('-', '_') и подмены фронтенда ('.', '+', '=').
// Используется для быстрой проверки гипотезы "похоже на base64", прежде чем пытаться декодировать.
// '+' и '=' — нестандартные для URL-safe, но могут встретиться в "прямом" base64 от внешних клиентов.
var base64LikeRegex = regexp.MustCompile(`^[A-Za-z0-9_.\-+=]*$`)

// Base64UrlDecode декодирует base64-url-encoded строку, как её отправляет фронтенд.
//
// В URL-safe base64 используются символы '-' и '_' вместо '+' и '/', а padding
// записывается символом '='. Фронтенд (см. frontend/src/helpers/encoding.js) делает
// дополнительную подстановку: '+' → '.', '/' → '_', '=' → '-'. То есть в URL может
// прийти что угодно — от "чистого" base64-URL до "смешанной" кодировки с подменой
// символов. Кроме того, некоторые бэкенды/инсталляции присылают plain-text ID
// (например, "wg0", "test-1") — их тоже нужно корректно пропускать.
//
// Баг 3: предыдущая реализация просто вызывала base64.URLEncoding.DecodeString
// на входе, что для строки "YXdnMA--" (фронтенд-кодирование "awg0") давало
// мусор "awg0\x0f\xbe" вместо "awg0" — Go base64.URLEncoding лояльно относится
// к нестандартным символам и не считает '-' ошибкой. Результат — PreparePeer
// не находил интерфейс и возвращал 500 "Failed to load prepared peer!".
//
// Первая попытка фикса (URLEncoding сначала, StdEncoding потом) тоже была
// неполной: для "YXdnMTA-" (фронтенд-кодирование "awg10", 7 символов с одним
// padding) URLEncoding возвращал "awg10>" — это печатный ASCII, поэтому
// isPrintableASCII пропускал его как валидный результат, и StdEncoding-путь
// уже не пробовался. Итог — "awg10" превращался в "awg10>" и снова не
// находил интерфейс в БД.
//
// Алгоритм:
//  1. Пустой вход или символы вне base64-алфавита — возвращаем как есть
//     (отсекает plain-text ID типа "wg0" / "test-1").
//  2. Пробуем frontend-кодирование ПЕРВЫМ: заменяем '-' → '=', '_' → '/', '.' → '+',
//     дополняем padding и декодируем StdEncoding. Это правильный путь для всех
//     строк, отправленных через base64_url_encode(), и не зависит от того,
//     считает ли Go нестандартные символы ошибкой.
//  3. Если шаг 2 не дал printable-ASCII результат — пробуем raw URL-safe (без padding),
//     на случай если клиент прислал чистый URL-encoded без padding.
//  4. Если ничего не сработало — возвращаем оригинал (plain-text ID, plain-ASCII).
func Base64UrlDecode(in string) string {
	if in == "" {
		return in
	}

	original := in

	// Шаг 1: отсекаем всё, что не похоже на base64 — это plain-text ID,
	// которые некоторые бэкенды/конфигурации отправляют без кодирования.
	if !base64LikeRegex.MatchString(in) {
		return original
	}

	// Шаг 2: frontend-кодирование ('-' вместо '=', '_' вместо '/', '.' вместо '+').
	// Это основной путь для всех URL-параметров, которые кодируются через
	// base64_url_encode() на фронтенде. Идём ПЕРВЫМ, потому что Go URLEncoding
	// лояльно относится к padding-символам и для строк вида "YXdnMA--" или
	// "YXdnMTA-" возвращает мусор ("awg0\x0f\xbe" / "awg10>"), который
	// проходит проверку isPrintableASCII — после такого "успеха" StdEncoding-путь
	// уже не вызывается и мы застреваем на неверном результате.
	inReplaced := strings.ReplaceAll(in, "-", "=")
	inReplaced = strings.ReplaceAll(inReplaced, "_", "/")
	inReplaced = strings.ReplaceAll(inReplaced, ".", "+")
	for len(inReplaced)%4 != 0 {
		inReplaced += "="
	}
	if decoded, err := base64.StdEncoding.DecodeString(inReplaced); err == nil {
		if s := string(decoded); isPrintableASCII(s) {
			return s
		}
	}

	// Шаг 3: raw URL-safe без padding — на случай, если клиент прислал
	// "чистый" URL-encoded (например, "d2cw" для "wg0").
	if decoded, err := base64.RawURLEncoding.DecodeString(in); err == nil {
		if s := string(decoded); isPrintableASCII(s) {
			return s
		}
	}

	// Шаг 4: ничего не сработало — отдаём plain-text как есть.
	return original
}

// isPrintableASCII возвращает true, если строка состоит только из печатных ASCII-символов
// (диапазон 0x20..0x7E). Используется, чтобы отсеять мусорные результаты base64-декодирования,
// когда Go радостно "декодирует" строку с нестандартными символами в байты с произвольными
// значениями (классический пример — "YXdnMA--" → "awg0\x0f\xbe" из бага 3).
func isPrintableASCII(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			return false
		}
	}
	return true
}