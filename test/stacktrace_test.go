package test

import (
	"log"
	"runtime/debug"
	"testing"
)

func b() {
	log.Println(string(debug.Stack()))
}

func a() {
	b()
}

func TestStackTrace(t *testing.T) {
	a()
}
