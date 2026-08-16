package main

import "testing"

func TestCalculator(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Errorf("expected 2 + 3 = 5")
	}
	if Subtract(10, 4) != 6 {
		t.Errorf("expected 10 - 4 = 6")
	}
	if Multiply(3, 4) != 12 {
		t.Errorf("expected 3 * 4 = 12")
	}
	res, err := Divide(10, 2)
	if err != nil || res != 5 {
		t.Errorf("expected 10 / 2 = 5, got %d (err: %v)", res, err)
	}
	_, err = Divide(10, 0)
	if err == nil {
		t.Errorf("expected error on division by zero")
	}
}
