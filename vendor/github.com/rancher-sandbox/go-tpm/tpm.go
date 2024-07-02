package tpm

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/google/go-attestation/attest"
	"github.com/google/go-tpm-tools/simulator"
	"github.com/pkg/errors"

	"github.com/rancher-sandbox/go-tpm/backend"
)

// GenerateChallenge generates a challenge from attestation data and a public endorsed key
func GenerateChallenge(ek *attest.EK, attestationData *AttestationData) ([]byte, []byte, error) {
	ap := attest.ActivationParameters{
		TPMVersion: attest.TPMVersion20,
		EK:         ek.Public,
		AK:         *attestationData.AK,
	}

	secret, ec, err := ap.Generate()
	if err != nil {
		return nil, nil, fmt.Errorf("generating challenge: %w", err)
	}

	challengeBytes, err := json.Marshal(Challenge{EC: ec})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling challenge: %w", err)
	}

	return secret, challengeBytes, nil
}

// ResolveToken is just syntax sugar around GetPubHash.
// If the token provided is in EK's form it just returns it, otherwise
// retrieves the pubhash
func ResolveToken(token string, opts ...Option) (bool, string, error) {
	if !strings.HasPrefix(token, "tpm://") {
		return false, token, nil
	}

	hash, err := GetPubHash(opts...)
	return true, hash, err
}

// GetPubHash returns the EK's pub hash
func GetPubHash(opts ...Option) (string, error) {
	c := &config{}
	c.apply(opts...)

	ek, err := getEK(c)
	if err != nil {
		return "", fmt.Errorf("getting EK: %w", err)
	}

	hash, err := DecodePubHash(ek)
	if err != nil {
		return "", fmt.Errorf("hashing EK: %w", err)
	}

	return hash, nil
}

func getTPM(c *config) (*attest.TPM, error) {

	cfg := &attest.OpenConfig{
		TPMVersion: attest.TPMVersion20,
	}
	if c.commandChannel != nil {
		cfg.CommandChannel = c.commandChannel
	}

	if c.emulated {
		var sim *simulator.Simulator
		var err error
		if c.seed != 0 {
			sim, err = simulator.GetWithFixedSeedInsecure(c.seed)
		} else {
			sim, err = simulator.Get()
		}
		if err != nil {
			return nil, err
		}
		cfg.CommandChannel = backend.Fake(sim)
	}

	return attest.OpenTPM(cfg)

}

func getEK(c *config) (*attest.EK, error) {
	var err error

	tpm, err := getTPM(c)
	if err != nil {
		return nil, fmt.Errorf("opening tpm for decoding EK: %w", err)
	}
	defer tpm.Close()

	eks, err := tpm.EKs()
	if err != nil {
		return nil, fmt.Errorf("getting eks: %w", err)
	}

	if len(eks) == 0 {
		return nil, fmt.Errorf("failed to find EK")
	}

	return &eks[0], nil
}

func getToken(data *AttestationData) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshalling attestation data: %w", err)
	}

	return "Bearer TPM" + base64.StdEncoding.EncodeToString(bytes), nil
}

func getAttestationData(c *config) (*AttestationData, []byte, error) {
	var err error

	tpm, err := getTPM(c)
	if err != nil {
		return nil, nil, fmt.Errorf("opening tpm for getting attestation data: %w", err)
	}
	defer tpm.Close()

	eks, err := tpm.EKs()
	if err != nil {
		return nil, nil, err
	}

	ak, err := tpm.NewAK(nil)
	if err != nil {
		return nil, nil, err
	}
	defer ak.Close(tpm)

	params := ak.AttestationParameters()

	if len(eks) == 0 {
		return nil, nil, fmt.Errorf("failed to find EK")
	}

	ek := &eks[0]
	ekBytes, err := encodeEK(ek)
	if err != nil {
		return nil, nil, err
	}

	aikBytes, err := ak.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling AK: %w", err)
	}

	return &AttestationData{
		EK: ekBytes,
		AK: &params,
	}, aikBytes, nil
}

// DecodeEK decodes EK pem bytes to attest.EK
func DecodeEK(pemBytes []byte) (*attest.EK, error) {
	block, _ := pem.Decode(pemBytes)

	if block == nil {
		return nil, errors.New("invalid pemBytes")
	}

	switch block.Type {
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing certificate: %v", err)
		}
		return &attest.EK{
			Certificate: cert,
			Public:      cert.PublicKey,
		}, nil

	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing ecdsa public key: %v", err)
		}

		return &attest.EK{
			Public: pub,
		}, nil
	}

	return nil, fmt.Errorf("invalid pem type: %s", block.Type)
}

// GetAttestationData returns attestation data from a TPM bearer token
func GetAttestationData(header string) (*attest.EK, *AttestationData, error) {
	tpmBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Bearer TPM"))
	if err != nil {
		return nil, nil, err
	}

	var attestationData AttestationData
	if err := json.Unmarshal(tpmBytes, &attestationData); err != nil {
		return nil, nil, err
	}

	ek, err := DecodeEK(attestationData.EK)
	if err != nil {
		return nil, nil, err
	}

	return ek, &attestationData, nil
}

// ValidateChallenge validates a challange against a secret
func ValidateChallenge(secret, resp []byte) error {
	var response ChallengeResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return fmt.Errorf("unmarshalling challenge response: %w", err)
	}
	if !bytes.Equal(secret, response.Secret) {
		return fmt.Errorf("invalid challenge response")
	}
	return nil
}
