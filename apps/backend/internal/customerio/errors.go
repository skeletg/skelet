package customerio

import "errors"

var (
	errInvalidEmail    = errors.New("email is missing or malformed")
	errChannelRequired = errors.New("customers belong to a channel - pick one for this run")
)
