package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type ids struct{ sequence atomic.Uint64 }

func (g *ids) NewID(prefix string) string {
	var raw [6]byte
	_, _ = rand.Read(raw[:])
	return prefix + "-" + hex.EncodeToString(raw[:]) + "-" + uintText(g.sequence.Add(1))
}
func uintText(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
