package drivers

import (
	"fmt"
	"hash/fnv"
)

func hex4(v uint16) string { return fmt.Sprintf("%04X", v) }

// shortID turns a device path into a short stable id, so two identical devices get
// different keys.
func shortID(path string) string {
	h := fnv.New32a()
	h.Write([]byte(path))
	return fmt.Sprintf("%08x", h.Sum32())
}
