package core

import (
	"errors"
	"testing"
)

var errNoLens = errors.New("no lens")

func TestLastReachResult(t *testing.T) {
	if _, err := lastReachResult([]Value{NewInteger(1)}, errNoLens); err != errNoLens {
		t.Errorf("an error must win, got %v", err)
	}
	if _, err := lastReachResult(nil, nil); err == nil {
		t.Error("an empty result must report that the lens produced no value")
	}
	got, err := lastReachResult([]Value{NewInteger(1), NewInteger(9)}, nil)
	if err != nil {
		t.Fatalf("lastReachResult: %v", err)
	}
	if s := got.String(); s != "9" {
		t.Errorf("the read is the chain's LAST value, got %s", s)
	}
}
