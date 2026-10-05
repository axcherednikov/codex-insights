package cache

type cacheError string

func (err cacheError) Error() string {
	return string(err)
}

const errUnsupportedCacheVersion cacheError = "cache version mismatch"
