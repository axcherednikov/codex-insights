package i18n

import (
	"fmt"
	"os"
	"strings"
)

type Language string

const (
	English Language = "en"
	Russian Language = "ru"
	Auto    Language = "auto"
)

type Translator struct {
	Language Language
}

func New(value string) (Translator, error) {
	language := Language(strings.ToLower(value))

	switch language {
	case "", Auto:
		language = detectLanguage()
	case English, Russian:
	default:
		return Translator{}, fmt.Errorf(
			"unsupported language %q; supported: auto, en, ru",
			value,
		)
	}

	return Translator{
		Language: language,
	}, nil
}

func detectLanguage() Language {
	for _, key := range []string{
		"LC_ALL",
		"LC_MESSAGES",
		"LANG",
	} {
		value := strings.ToLower(os.Getenv(key))

		if strings.HasPrefix(value, "ru") {
			return Russian
		}

		if strings.HasPrefix(value, "en") {
			return English
		}
	}

	return English
}

func (t Translator) IsRussian() bool {
	return t.Language == Russian
}
