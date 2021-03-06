package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/net/context"

	kubelego "github.com/harborfront/kube-lego/pkg/kubelego_const"
)

func (a *Acme) getContact() []string {
	return []string{
		fmt.Sprintf("mailto:%s", strings.ToLower(a.kubelego.LegoEmail())),
	}
}

func (a *Acme) acceptTos(tos string) bool {
	a.Log().Infof("if you don't accept the TOS (%s) please exit the program now", tos)
	return true
}

func (a *Acme) createUser() (client *acme.Client, account *acme.Account, err error) {
	privateKeyPem, privateKey, err := a.generatePrivateKey()
	if err != nil {
		return nil, nil, err
	}

	client = &acme.Client{
		Key:          privateKey,
		DirectoryURL: a.kubelego.LegoURL(),
	}

	account = &acme.Account{
		Contact: a.getContact(),
	}

	account, err = client.Register(
		context.Background(),
		account,
		a.acceptTos,
	)
	if err != nil {
		return nil, nil, err
	}
	a.Log().Infof("created an ACME account (registration url: %s)", account.URI)

	err = a.kubelego.SaveAcmeUser(
		map[string][]byte{
			kubelego.AcmePrivateKey:      privateKeyPem,
			kubelego.AcmeRegistrationUrl: []byte(account.URI),
		},
	)
	if err != nil {
		return nil, nil, err
	}

	return client, account, err
}

func (a *Acme) getUser() (client *acme.Client, accountURI string, err error) {

	userData, err := a.kubelego.AcmeUser()
	if err != nil {
		return nil, "", err
	}

	privateKeyData, ok := userData[kubelego.AcmePrivateKey]
	if !ok {
		return nil, "", fmt.Errorf("could not find acme private key with key '%s'", kubelego.AcmePrivateKey)
	}
	block, _ := pem.Decode(privateKeyData)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", err
	}
	client = &acme.Client{
		Key:          privateKey,
		DirectoryURL: a.kubelego.LegoURL(),
	}

	accountURIBytes, ok := userData[kubelego.AcmeRegistrationUrl]
	if ok {
		return client, string(accountURIBytes), nil
	}

	regData, ok := userData[kubelego.AcmeRegistration]
	if !ok {
		return nil, "", fmt.Errorf("could not find an ACME account URI in the account secret")
	}
	reg := acmeAccountRegistration{}
	err = json.Unmarshal(regData, &reg)
	if err != nil {
		return nil, "", err
	}

	return client, reg.URI, nil
}

func (a *Acme) validateUser(client *acme.Client, accountURI string) (account *acme.Account, err error) {

	account, err = client.GetReg(context.Background(), accountURI)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ACME account for URI '%s': %s", accountURI, err)
	}

	contact := a.getContact()
	if !reflect.DeepEqual(account.Contact, contact) {
		account.Contact = contact
		account, err = client.UpdateReg(context.Background(), account)
		if err != nil {
			return nil, fmt.Errorf("failed to update ACME account's contact to '%s': %s", contact, err)
		}
		a.Log().Infof("updated ACME account's contact to '%s'", contact)
	}

	return account, nil
}

func (a *Acme) generatePrivateKey() ([]byte, crypto.Signer, error) {

	if a.kubelego.LegoKeyType() == kubelego.KeyTypeRsa {

		privateKey, err := rsa.GenerateKey(rand.Reader, a.kubelego.LegoKeySize())
		if err != nil {
			return []byte{}, nil, err
		}

		block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}

		return pem.EncodeToMemory(block), privateKey, nil
	}

	var ecpk *ecdsa.PrivateKey
	var err error

	switch a.kubelego.LegoKeySize() {
	case 224:
		ecpk, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
		break

	case 256:
		ecpk, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		break

	default:
		ecpk, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		break
		
	case 384:
		ecpk, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		break

	case 521:
		ecpk, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		break

	}

	if err != nil {
		return []byte{}, nil, err
	}

	b, err := x509.MarshalECPrivateKey(ecpk)
	if err != nil {
		return []byte{}, nil, err
	}

	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}

	return pem.EncodeToMemory(block), ecpk, nil
}
