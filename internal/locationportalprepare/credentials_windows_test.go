//go:build windows && (amd64 || arm64)

package locationportalprepare

import (
	"bytes"
	"errors"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"golang.org/x/sys/windows"
	"reflect"
	"testing"
	"unsafe"
)

type fakeBox struct {
	lastPlain, lastEntropy     []byte
	failProtect, failUnprotect bool
	freeCalls                  int
}

func (f *fakeBox) funcs() dpapiFunctions {
	return dpapiFunctions{
		protect: func(in, entropy *windows.DataBlob) ([]byte, error) {
			if f.failProtect {
				return nil, errors.New("protect")
			}
			f.lastPlain = append([]byte{}, unsafe.Slice(in.Data, in.Size)...)
			f.lastEntropy = append([]byte{}, unsafe.Slice(entropy.Data, entropy.Size)...)
			return append([]byte("box:"), f.lastPlain...), nil
		},
		unprotect: func(in, entropy *windows.DataBlob) ([]byte, error) {
			if f.failUnprotect {
				return nil, errors.New("unprotect")
			}
			if !bytes.Equal(f.lastEntropy, unsafe.Slice(entropy.Data, entropy.Size)) {
				return nil, errors.New("entropy")
			}
			return bytes.TrimPrefix(unsafe.Slice(in.Data, in.Size), []byte("box:")), nil
		},
		free: func(v []byte) error {
			f.freeCalls++
			for i := range v {
				v[i] = 0
			}
			return nil
		},
	}
}
func TestCredentialBoxBindsOwnerTransactionAndPinWithoutConfigMarshal(t *testing.T) {
	owner := "S-1-5-21-42"
	tx := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	pin := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	c := locationportal.Config{Port: HTTPSPort, CertificateSHA256: pin, Username: portalSecretUser, Password: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	f := &fakeBox{}
	box, e := sealCredentialConfigWith(f.funcs(), c, owner, tx, pin)
	if e != nil {
		t.Fatal(e)
	}
	if len(box) == 0 || f.freeCalls != 2 {
		t.Fatal("plaintext/entropy not cleared")
	}
	got, e := unsealCredentialConfigWith(f.funcs(), box, owner, tx, pin)
	if e != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("%#v %v", got, e)
	}
	if _, e = unsealCredentialConfigWith(f.funcs(), box, owner, tx, pin[:62]+"aa"); e == nil {
		t.Fatal("wrong pin accepted")
	}
	if _, e = unsealCredentialConfigWith(f.funcs(), box, owner, tx[:62]+"aa", pin); e == nil {
		t.Fatal("wrong transaction accepted")
	}
	if _, e = unsealCredentialConfigWith(f.funcs(), box, owner+"0", tx, pin); e == nil {
		t.Fatal("wrong owner accepted")
	}
}
func TestCredentialBoxSupportsPreCertificateSealOnlyInMemory(t *testing.T) {
	owner := "S-1-5-21-42"
	tx := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	c := locationportal.Config{Port: HTTPSPort, Username: portalSecretUser, Password: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	f := &fakeBox{}
	box, e := sealCredentialConfigWith(f.funcs(), c, owner, tx, "")
	if e != nil {
		t.Fatal(e)
	}
	got, e := unsealCredentialConfigWith(f.funcs(), box, owner, tx, "")
	if e != nil || got.CertificateSHA256 != "" {
		t.Fatal(got, e)
	}
}
func TestCredentialBoxRejectsPlainConfigAliasAndOversize(t *testing.T) {
	owner := "S-1-5-21-42"
	tx := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	pin := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	c := locationportal.Config{Port: HTTPSPort, CertificateSHA256: pin, Username: portalSecretUser, Password: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	f := &fakeBox{}
	box, e := sealCredentialConfigWith(f.funcs(), c, owner, tx, pin)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = unmarshalCredential([]byte(`{"Version":1}`), owner, tx, pin); e == nil {
		t.Fatal("alias accepted")
	}
	if _, e = unsealCredentialConfigWith(f.funcs(), append(box, make([]byte, portalSecretMax)...), owner, tx, pin); e == nil {
		t.Fatal("oversized box accepted")
	}
}
