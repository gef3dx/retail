package gismt

import (
	"crypto/sha256"
	"fmt"
)

// Mock-провайдер ГИС МТ («Честный знак») для этапов 5-8. Настоящий API — этап 9.

func Send(operation string, codeID int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("mock-gismt:%s:%d", operation, codeID)))
	return fmt.Sprintf("GISMT-%x", h)[:20]
}
