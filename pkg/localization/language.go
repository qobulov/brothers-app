// Package localization resolves the supported response language from HTTP headers.
package localization

import (
	"strconv"
	"strings"
)

const DefaultLanguage = "en"

// HeaderApplicationLanguage is the plain language code the mobile app sends.
const HeaderApplicationLanguage = "Application-Language"

// RequestLanguage prefers the app's Application-Language header when it holds a
// supported language, and otherwise falls back to Accept-Language.
func RequestLanguage(applicationLanguage, acceptLanguage string) string {
	language := strings.SplitN(strings.ToLower(strings.TrimSpace(applicationLanguage)), "-", 2)[0]
	switch language {
	case "uz", "ru", "en":
		return language
	}
	return ResolveAcceptLanguage(acceptLanguage)
}

// ResolveAcceptLanguage selects the supported language with the greatest q-value
// from an Accept-Language header. Equal priorities retain the header order.
func ResolveAcceptLanguage(header string) string {
	selected := DefaultLanguage
	bestQuality := -1.0

	for _, value := range strings.Split(strings.ToLower(header), ",") {
		parts := strings.Split(value, ";")
		language := strings.SplitN(strings.TrimSpace(parts[0]), "-", 2)[0]
		if language != "uz" && language != "ru" && language != "en" {
			continue
		}

		quality := 1.0
		for _, parameter := range parts[1:] {
			key, rawQuality, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(key, "q") {
				continue
			}
			parsedQuality, err := strconv.ParseFloat(rawQuality, 64)
			if err != nil || parsedQuality < 0 || parsedQuality > 1 {
				quality = 0
				break
			}
			quality = parsedQuality
		}

		if quality == 0 {
			continue
		}
		if quality > bestQuality {
			selected, bestQuality = language, quality
		}
	}

	return selected
}
