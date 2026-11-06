package service

import "unsafe"

// requestJSONReadOnlyString borrows raw only for synchronous parsing. Returned
// field values that outlive that scan must own their storage.
func requestJSONReadOnlyString(raw []byte) string {
	return unsafe.String(unsafe.SliceData(raw), len(raw))
}
