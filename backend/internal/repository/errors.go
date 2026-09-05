package repository

import (
	"errors"
	"fmt"
	"strconv"
)

// errNotFound — код/сущность не найдены (сервис мапит в 404).
var errNotFound = errors.New("not found")

// errConflict создает ошибку конфликта с сообщением (сервис мапит в 409).
func errConflict(msg string) error { return fmt.Errorf("conflict: %s", msg) }

// IsConflict проверяет конфликтную ошибку репозитория.
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrAlreadyPosted || err == ErrInsufficientStock || err == ErrNoStock ||
		len(err.Error()) >= 9 && err.Error()[:9] == "conflict:"
}

// itoa — strconv.Itoa для построения $N плейсхолдеров.
func itoa(i int) string { return strconv.Itoa(i) }
