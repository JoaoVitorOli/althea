package sdk

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// NewID returns a new unique ID for a record an extension keeps, as a note or a link. IDs sort
// by the time they were made; a random part keeps two made at the same instant apart, as the
// system clock can give the same time twice.
func NewID() string {
	var r [5]byte
	_, _ = rand.Read(r[:]) // crypto/rand.Read never fails
	return strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + hex.EncodeToString(r[:])
}
