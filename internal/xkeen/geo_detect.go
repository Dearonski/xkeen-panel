package xkeen

import (
	"strings"
	"unicode"
)

// detectCountry определяет ISO-код страны по имени сервера: сначала флаг-эмодзи,
// затем словарь ключевых слов. Возвращает "" если распознать не удалось.
// Это вспомогательный/косметический сигнал — основной источник страны GeoIP.
func detectCountry(name string) string {
	if c := countryFromFlag(name); c != "" {
		return c
	}

	lower := strings.ToLower(name)

	for _, tok := range strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if code, ok := countryTokens[tok]; ok {
			return code
		}
	}

	for _, k := range countrySubstrings {
		if strings.Contains(lower, k.word) {
			return k.code
		}
	}

	return ""
}

// countryFromFlag извлекает ISO-код из флаг-эмодзи (пара regional indicator символов).
func countryFromFlag(s string) string {
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x1F1E6 && a <= 0x1F1FF && b >= 0x1F1E6 && b <= 0x1F1FF {
			return string([]rune{'A' + (a - 0x1F1E6), 'A' + (b - 0x1F1E6)})
		}
	}
	return ""
}

// countryTokens — точное совпадение по токену (безопасно для коротких кодов вроде "ru").
var countryTokens = map[string]string{
	"ru": "RU", "rus": "RU", "russia": "RU", "россия": "RU", "ру": "RU", "рф": "RU",
	"moscow": "RU", "москва": "RU", "msk": "RU", "spb": "RU", "питер": "RU",
	"by": "BY", "blr": "BY", "belarus": "BY", "беларусь": "BY", "minsk": "BY", "минск": "BY",
	"nl": "NL", "nld": "NL", "netherlands": "NL", "нидерланды": "NL", "amsterdam": "NL", "амстердам": "NL",
	"de": "DE", "ger": "DE", "deu": "DE", "germany": "DE", "германия": "DE", "frankfurt": "DE", "франкфурт": "DE",
	"us": "US", "usa": "US", "сша": "US", "америка": "US",
	"gb": "GB", "uk": "GB", "england": "GB", "britain": "GB", "london": "GB", "лондон": "GB",
	"fr": "FR", "fra": "FR", "france": "FR", "франция": "FR", "paris": "FR", "париж": "FR",
	"fi": "FI", "fin": "FI", "finland": "FI", "финляндия": "FI", "helsinki": "FI", "хельсинки": "FI",
	"se": "SE", "swe": "SE", "sweden": "SE", "швеция": "SE", "stockholm": "SE",
	"ua": "UA", "ukraine": "UA", "украина": "UA", "kyiv": "UA", "kiev": "UA", "киев": "UA",
	"pl": "PL", "pol": "PL", "poland": "PL", "польша": "PL", "warsaw": "PL", "варшава": "PL",
	"tr": "TR", "tur": "TR", "turkey": "TR", "турция": "TR", "istanbul": "TR", "стамбул": "TR",
	"jp": "JP", "jpn": "JP", "japan": "JP", "япония": "JP", "tokyo": "JP", "токио": "JP",
	"sg": "SG", "sgp": "SG", "singapore": "SG", "сингапур": "SG",
	"hk": "HK", "hongkong": "HK", "гонконг": "HK",
	"kz": "KZ", "kazakhstan": "KZ", "казахстан": "KZ",
	"ae": "AE", "uae": "AE", "dubai": "AE", "дубай": "AE", "эмираты": "AE",
	"ch": "CH", "che": "CH", "switzerland": "CH", "швейцария": "CH", "zurich": "CH",
	"at": "AT", "aut": "AT", "austria": "AT", "австрия": "AT", "vienna": "AT", "вена": "AT",
	"es": "ES", "esp": "ES", "spain": "ES", "испания": "ES", "madrid": "ES",
	"it": "IT", "ita": "IT", "italy": "IT", "италия": "IT", "milan": "IT", "милан": "IT", "rome": "IT",
	"ca": "CA", "can": "CA", "canada": "CA", "канада": "CA",
	"lv": "LV", "latvia": "LV", "латвия": "LV", "riga": "LV", "рига": "LV",
	"lt": "LT", "lithuania": "LT", "литва": "LT", "vilnius": "LT",
	"ee": "EE", "estonia": "EE", "эстония": "EE", "tallinn": "EE", "таллин": "EE",
	"no": "NO", "norway": "NO", "норвегия": "NO",
	"dk": "DK", "denmark": "DK", "дания": "DK",
	"cz": "CZ", "czech": "CZ", "чехия": "CZ", "prague": "CZ", "прага": "CZ",
}

// countrySubstrings — для имён без разделителей (substring), только однозначные длинные слова.
var countrySubstrings = []struct{ word, code string }{
	{"russia", "RU"}, {"россия", "RU"}, {"moscow", "RU"}, {"москва", "RU"},
	{"belarus", "BY"}, {"беларус", "BY"}, {"minsk", "BY"},
	{"netherland", "NL"}, {"нидерланд", "NL"}, {"amsterdam", "NL"},
	{"germany", "DE"}, {"германия", "DE"}, {"frankfurt", "DE"},
	{"finland", "FI"}, {"финлянд", "FI"}, {"helsinki", "FI"},
	{"france", "FR"}, {"франция", "FR"},
	{"sweden", "SE"}, {"швеция", "SE"},
	{"singapore", "SG"}, {"сингапур", "SG"},
	{"turkey", "TR"}, {"турция", "TR"},
}
