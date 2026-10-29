//go:build windows && (amd64 || arm64)

package locationportalprepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"golang.org/x/sys/windows"
)

const portalSecretUser = "ultralocation"
const portalSecretMax = 4096

type privateCredentialWire struct {
	Version     int    `json:"version"`
	Owner       string `json:"owner"`
	Transaction string `json:"transaction"`
	Pin         string `json:"pin"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type dpapiFunctions struct {
	protect   func(*windows.DataBlob, *windows.DataBlob) ([]byte, error)
	unprotect func(*windows.DataBlob, *windows.DataBlob) ([]byte, error)
	free      func([]byte) error
}

var nativeDPAPI = dpapiFunctions{
	protect: func(in, entropy *windows.DataBlob) ([]byte, error) {
		var out windows.DataBlob
		if err := windows.CryptProtectData(in, nil, entropy, 0, nil, windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
			return nil, err
		}
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
		if out.Data == nil || out.Size == 0 || out.Size > portalSecretMax {
			return nil, ErrReadback
		}
		return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
	},
	unprotect: func(in, entropy *windows.DataBlob) ([]byte, error) {
		var out windows.DataBlob
		if err := windows.CryptUnprotectData(in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
			return nil, err
		}
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
		if out.Data == nil || out.Size == 0 || out.Size > portalSecretMax {
			return nil, ErrReadback
		}
		return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
	},
	free: func(data []byte) error {
		for i := range data {
			data[i] = 0
		}
		return nil
	},
}

func credentialEntropy(owner, transaction, pin string) []byte {
	h := sha256.New()
	h.Write([]byte("godusevpn-ultra/location-portal/credential-box/v1\x00"))
	h.Write([]byte(owner))
	h.Write([]byte{0})
	h.Write([]byte(transaction))
	h.Write([]byte{0})
	h.Write([]byte(pin))
	return h.Sum(nil)
}
func validCredentialPin(pin string) bool     { return pin == "" || validPin(pin) }
func validCredentialOwner(owner string) bool { return validOwnerSID(owner) }
func validCredentialTransaction(tx string) bool {
	return len(tx) == 64 && strings.ToLower(tx) == tx && func() bool { _, e := hex.DecodeString(tx); return e == nil }()
}
func validCredential(c locationportal.Config, pin string) bool {
	if c.Port != HTTPSPort || c.Username != portalSecretUser || len(c.Password) != 64 || strings.ToLower(c.Password) != c.Password {
		return false
	}
	if pin == "" {
		return c.CertificateSHA256 == ""
	}
	return validPin(pin) && c.CertificateSHA256 == pin
}
func marshalCredential(c locationportal.Config, owner, transaction, pin string) ([]byte, error) {
	if !validCredential(c, pin) || !validCredentialOwner(owner) || !validCredentialTransaction(transaction) {
		return nil, ErrReadback
	}
	w := privateCredentialWire{1, owner, transaction, pin, c.Port, c.Username, c.Password}
	return json.Marshal(w)
}
func unmarshalCredential(raw []byte, owner, transaction, pin string) (locationportal.Config, error) {
	var out locationportal.Config
	var w privateCredentialWire
	if len(raw) == 0 || len(raw) > portalSecretMax || !validCredentialOwner(owner) || !validCredentialTransaction(transaction) || !validCredentialPin(pin) {
		return out, ErrReadback
	}
	if exactJSON(raw, &w) != nil || w.Version != 1 || w.Owner != owner || w.Transaction != transaction || w.Pin != pin || w.Port != HTTPSPort || w.Username != portalSecretUser || len(w.Password) != 64 || strings.ToLower(w.Password) != w.Password {
		return out, ErrReadback
	}
	out = locationportal.Config{Port: w.Port, CertificateSHA256: w.Pin, Username: w.Username, Password: w.Password}
	if !validCredential(out, pin) {
		return locationportal.Config{}, ErrReadback
	}
	return out, nil
}
func blobOf(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func sealCredentialConfigWith(f dpapiFunctions, c locationportal.Config, owner, transaction, pin string) ([]byte, error) {
	plain, e := marshalCredential(c, owner, transaction, pin)
	if e != nil {
		return nil, e
	}
	defer f.free(plain)
	entropy := credentialEntropy(owner, transaction, pin)
	defer f.free(entropy)
	protected, e := f.protect(blobOf(plain), blobOf(entropy))
	if e != nil {
		return nil, e
	}
	return protected, nil
}
func unsealCredentialConfigWith(f dpapiFunctions, box []byte, owner, transaction, pin string) (locationportal.Config, error) {
	if len(box) == 0 || len(box) > portalSecretMax {
		return locationportal.Config{}, ErrReadback
	}
	entropy := credentialEntropy(owner, transaction, pin)
	defer f.free(entropy)
	plain, e := f.unprotect(blobOf(box), blobOf(entropy))
	if e != nil {
		return locationportal.Config{}, e
	}
	defer f.free(plain)
	return unmarshalCredential(plain, owner, transaction, pin)
}
func nativeSealConfig(_ context.Context, c locationportal.Config, owner, transaction, pin string) ([]byte, error) {
	return sealCredentialConfigWith(nativeDPAPI, c, owner, transaction, pin)
}
func nativeUnsealConfig(_ context.Context, box []byte, owner, transaction, pin string) (locationportal.Config, error) {
	return unsealCredentialConfigWith(nativeDPAPI, box, owner, transaction, pin)
}
