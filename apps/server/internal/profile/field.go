package profile

type FieldError struct {
	Field string
	Err   error
}

func (e FieldError) Error() string {
	return e.Err.Error()
}

func (e FieldError) Unwrap() error {
	return e.Err
}
