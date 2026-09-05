package service

// Error — типовая ошибка сервиса с HTTP-кодом. Хендлеры мапят её в JSON.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func BadRequest(msg string) *Error   { return &Error{Code: 400, Msg: msg} }
func Unauthorized(msg string) *Error { return &Error{Code: 401, Msg: msg} }
func Forbidden(msg string) *Error    { return &Error{Code: 403, Msg: msg} }
func NotFound(msg string) *Error     { return &Error{Code: 404, Msg: msg} }
func Conflict(msg string) *Error     { return &Error{Code: 409, Msg: msg} }
func Internal(msg string) *Error     { return &Error{Code: 500, Msg: msg} }
