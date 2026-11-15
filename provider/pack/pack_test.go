package pack_test

import (
	"bytes"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider/pack"
)

func TestZipRoundTrip(t *testing.T) {
	z, err := pack.Zip("a.xml", []byte("<a/>"))
	if err != nil || !pack.IsZip(z) {
		t.Fatal(err)
	}
	name, data, err := pack.UnzipOne(z)
	if err != nil || name != "a.xml" || !bytes.Equal(data, []byte("<a/>")) {
		t.Fatalf("%s %s %v", name, data, err)
	}
	b, err := pack.FromBase64(pack.Base64(z)[:len(pack.Base64(z))-1] + "\n")
	if err != nil && !bytes.Equal(b, z) {
		t.Log("padding stripped variant", err)
	}
	b, err = pack.FromBase64(pack.Base64(z))
	if err != nil || !bytes.Equal(b, z) {
		t.Fatal("base64")
	}
	if pack.MD5Hex([]byte("a")) != "0cc175b9c0f1b6a831c399e269772661" {
		t.Fatal("md5")
	}
}
