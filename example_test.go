package ubltr_test

import (
	"fmt"

	"github.com/furkanmeclis/go-ubltr"
)

func ExampleNewInvoice() {
	inv := ubltr.NewInvoice()
	out, err := ubltr.Marshal(inv)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(len(out) > 0)
	// Output: true
}
