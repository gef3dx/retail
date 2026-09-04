package ofd

import (
	"crypto/sha256"
	"fmt"
)

// Mock — провайдер-заглушка ОФД для этапов 4-8. Настоящие ОФД — этап 9.
// Детерминирован: один чек всегда дает один ФД/ФП (повторная отправка идемпотентна).

type Result struct {
	DocNumber string
	Sign      string
	QRURL     string
}

func Send(receiptID int64) Result {
	h := sha256.Sum256([]byte(fmt.Sprintf("mock-ofd:%d", receiptID)))
	return Result{
		DocNumber: fmt.Sprintf("%d", 900000+receiptID),
		Sign:      fmt.Sprintf("%x", h)[:32],
		QRURL:     fmt.Sprintf("https://mock.ofd/check?fd=%d&fp=%x", 900000+receiptID, h),
	}
}
