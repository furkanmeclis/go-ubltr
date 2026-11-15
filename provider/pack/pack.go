// Package pack holds the zip / base64 / hash helpers that SOAP integrators
// expect around UBL-TR payloads.
package pack

import (
	"archive/zip"
	"bytes"
	"crypto/md5" //nolint:gosec // integrators require MD5 checksums, not for security
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

// Zip stores one file named name in a zip archive.
func Zip(name string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.Modified = time.Unix(0, 0).UTC()
	w, err := zw.CreateHeader(h)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MaxUnzipSize limits a single unzipped entry (64 MiB).
const MaxUnzipSize = 64 << 20

// Unzip returns every regular file in the archive keyed by name.
func Unzip(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pack: unzip: %w", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxUnzipSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(b) > MaxUnzipSize {
			return nil, errors.New("pack: zip entry too large")
		}
		out[f.Name] = b
	}
	return out, nil
}

// UnzipOne returns the only file in the archive.
func UnzipOne(data []byte) (string, []byte, error) {
	files, err := Unzip(data)
	if err != nil {
		return "", nil, err
	}
	if len(files) != 1 {
		return "", nil, fmt.Errorf("pack: want one file in zip, got %d", len(files))
	}
	for n, b := range files {
		return n, b, nil
	}
	return "", nil, nil
}

// Base64 encodes with standard padding.
func Base64(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

// FromBase64 decodes standard base64, tolerating missing padding and
// whitespace.
func FromBase64(s string) ([]byte, error) {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
		default:
			clean = append(clean, s[i])
		}
	}
	if b, err := base64.StdEncoding.DecodeString(string(clean)); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(string(bytes.TrimRight(clean, "=")))
}

// MD5Hex returns the lowercase hex MD5 of data.
func MD5Hex(data []byte) string {
	s := md5.Sum(data) //nolint:gosec // checksum required by integrator APIs
	return hex.EncodeToString(s[:])
}

// SHA256Hex returns the lowercase hex SHA-256 of data.
func SHA256Hex(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// IsZip reports whether data starts with a zip local-file signature.
func IsZip(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 3 && data[3] == 4
}
