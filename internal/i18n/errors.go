package i18n

type i18nError string

func (err i18nError) Error() string {
	return string(err)
}

const errUnsupportedLanguage i18nError = "unsupported language"
