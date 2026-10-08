package tpm

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-attestation/attest"
	"github.com/google/go-tpm-tools/simulator"
	gotpm2 "github.com/google/go-tpm/legacy/tpm2"
	"github.com/google/go-tpm/tpmutil"
	"github.com/pkg/errors"

	"github.com/rancher-sandbox/go-tpm/backend"
)

// GenerateChallenge generates a challenge from attestation data and a public endorsed key
func GenerateChallenge(ek *attest.EK, attestationData *AttestationData) ([]byte, []byte, error) {
	ap := attest.ActivationParameters{
		EK: ek.Public,
		AK: *attestationData.AK,
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

// srkHandle is the standard persistent handle for the Storage Root Key.
const srkHandleValue = tpmutil.Handle(0x81000001)

// defaultRSASRKTemplate matches go-attestation's internal SRK template.
var defaultRSASRKTemplate = gotpm2.Public{
	Type:       gotpm2.AlgRSA,
	NameAlg:    gotpm2.AlgSHA256,
	Attributes: gotpm2.FlagStorageDefault | gotpm2.FlagNoDA,
	RSAParameters: &gotpm2.RSAParams{
		Symmetric: &gotpm2.SymScheme{
			Alg:     gotpm2.AlgAES,
			KeyBits: 128,
			Mode:    gotpm2.AlgCFB,
		},
		ModulusRaw: make([]byte, 256),
		KeyBits:    2048,
	},
}

// provisionSRK creates and persists an RSA SRK under the Owner hierarchy, if
// one is not already present.
// This is to workaround the go-attestation SRK-creation fallback logic which
// uses the Endorsement hierarchy.
func provisionSRK(rwc io.ReadWriter) error {
	if _, _, _, err := gotpm2.ReadPublic(rwc, srkHandleValue); err == nil {
		return nil
	}
	keyHnd, _, err := gotpm2.CreatePrimary(rwc, gotpm2.HandleOwner, gotpm2.PCRSelection{}, "", "", defaultRSASRKTemplate)
	if err != nil {
		return fmt.Errorf("CreatePrimary: %w", err)
	}
	defer gotpm2.FlushContext(rwc, keyHnd)
	if err := gotpm2.EvictControl(rwc, "", gotpm2.HandleOwner, keyHnd, srkHandleValue); err != nil {
		return fmt.Errorf("EvictControl: %w", err)
	}
	return nil
}

func getTPM(c *config) (*attest.TPM, error) {

	cfg := &attest.OpenConfig{}
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
		// Pre-provision the SRK under the Owner hierarchy before handing the
		// simulator to go-attestation. Without this, go-attestation falls back
		// to creating the SRK internally via a code path that uses
		// HandleEndorsement (wrong hierarchy), producing a different key
		// across simulator sessions with the same seed.
		if err := provisionSRK(sim); err != nil {
			_ = sim.Close()
			return nil, fmt.Errorf("provisioning SRK: %w", err)
		}
		cfg.CommandChannel = backend.Fake(sim)
	}

	return attest.OpenTPM(cfg)

}

func getEK(c *config) (*attest.EK, error) {
	tpm, err := getTPM(c)
	if err != nil {
		return nil, fmt.Errorf("opening tpm for decoding EK: %w", err)
	}
	defer tpm.Close()
	return tpmGetEK(tpm)
}

func tpmGetEK(tpm *attest.TPM) (*attest.EK, error) {
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
	tpm, err := getTPM(c)
	if err != nil {
		return nil, nil, fmt.Errorf("opening tpm for getting attestation data: %w", err)
	}
	defer tpm.Close()
	return tpmGetAttestationData(tpm)
}

func tpmGetAttestationData(tpm *attest.TPM) (*AttestationData, []byte, error) {
	eks, err := tpm.EKs()
	if err != nil {
		return nil, nil, err
	}

	if len(eks) == 0 {
		return nil, nil, fmt.Errorf("failed to find EK")
	}

	ak, err := tpm.NewAK(nil)
	if err != nil {
		return nil, nil, err
	}
	defer ak.Close(tpm)

	params := ak.AttestationParameters()

	ekBytes, err := encodeEK(&eks[0])
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
