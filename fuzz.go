// +build gofuzz

package dcache

import (
	"github.com/coredns/coredns/plugin/pkg/fuzz"
)

// Fuzz fuzzes cache.
func Fuzz(data []byte) int {
	w := Dcache{}
	return fuzz.Do(w, data)
}
