package usecase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MediaTTL is how long a media link stays valid.
const MediaTTL = 10 * time.Minute

// Media kinds of a link.
const (
	MediaPhoto     = "photo"
	MediaSignature = "signature"
)

// MediaSigner signs short-lived media links: the token names the
// organization, the kind and the record, and expires; the API streams the
// object only after verifying it (the browser never sees an object key or
// a storage host).
type MediaSigner struct {
	key []byte
	now func() time.Time
}

// NewMediaSigner derives the link key from a server secret.
func NewMediaSigner(secret string) *MediaSigner {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("rentopia/inspection-media/v1"))
	return &MediaSigner{key: mac.Sum(nil), now: time.Now}
}

type mediaClaims struct {
	Org  int64     `json:"o"`
	Kind string    `json:"k"`
	ID   uuid.UUID `json:"u"`
	Exp  int64     `json:"e"`
}

// MediaTarget is what a verified token names.
type MediaTarget struct {
	OrgID int64
	Kind  string
	ID    uuid.UUID
}

// Sign returns a token and its expiry.
func (m *MediaSigner) Sign(orgID int64, kind string, id uuid.UUID) (string, time.Time) {
	exp := m.now().Add(MediaTTL).UTC().Truncate(time.Second)
	payload, _ := json.Marshal(mediaClaims{Org: orgID, Kind: kind, ID: id, Exp: exp.Unix()})
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + m.sig(p), exp
}

// Verify checks the signature and the expiry.
func (m *MediaSigner) Verify(token string) (MediaTarget, error) {
	p, sig, ok := strings.Cut(token, ".")
	if !ok || p == "" || sig == "" {
		return MediaTarget{}, ErrMediaLink
	}
	if !hmac.Equal([]byte(sig), []byte(m.sig(p))) {
		return MediaTarget{}, ErrMediaLink
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return MediaTarget{}, ErrMediaLink
	}
	var c mediaClaims
	if err := json.Unmarshal(raw, &c); err != nil || c.Org <= 0 || c.ID == uuid.Nil {
		return MediaTarget{}, ErrMediaLink
	}
	if m.now().Unix() > c.Exp {
		return MediaTarget{}, errors.Join(ErrMediaLink, errors.New("expired"))
	}
	if c.Kind != MediaPhoto && c.Kind != MediaSignature {
		return MediaTarget{}, ErrMediaLink
	}
	return MediaTarget{OrgID: c.Org, Kind: c.Kind, ID: c.ID}, nil
}

func (m *MediaSigner) sig(payload string) string {
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// MediaPath is the API path of a token (the browser prefixes the BFF).
func MediaPath(token string) string { return "/v1/public/media/inspections/" + token }
