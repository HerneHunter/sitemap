package sitemap

import (
	"errors"
	"fmt"
)

var (
	ErrUndeterminedType = errors.New("could not determine document type")
	ErrMalformedXML     = errors.New("malformed xml")
)

type TimeParseError struct {
	Raw string
}

func (e *TimeParseError) Error() string {
	return "failed to parse time: " + e.Raw
}

type HTTPError struct {
	URL        string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: unexpected http status code: %d", e.URL, e.StatusCode)
}
